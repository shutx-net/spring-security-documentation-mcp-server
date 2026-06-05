import {
    Stack,
    StackProps,
    Duration,
    RemovalPolicy,
    SecretValue,
    CfnOutput,
    aws_codebuild as codebuild,
    aws_ec2 as ec2,
    aws_ecr as ecr,
    aws_ecs as ecs,
    aws_iam as iam,
    aws_lambda as lambda,
    aws_logs as logs,
    aws_s3 as s3,
    aws_scheduler as scheduler,
    aws_stepfunctions as sfn,
    aws_stepfunctions_tasks as tasks,
    aws_s3vectors as s3vectors,
} from "aws-cdk-lib";
import { Construct } from "constructs";
import { AppConfig } from "./config";
import { DocTables } from "./storage-stack";

export interface PipelineStackProps extends StackProps {
    readonly config: AppConfig;
    readonly vpc: ec2.IVpc;
    readonly contentBucket: s3.IBucket;
    readonly vectorBucket: s3vectors.CfnVectorBucket;
    readonly vectorIndex: s3vectors.CfnIndex;
    readonly tables: DocTables;
    readonly chunksTableGsiName: string;
    readonly indexerRepository: ecr.IRepository;
}

export class PipelineStack extends Stack {
    public readonly stateMachine: sfn.StateMachine;

    constructor(scope: Construct, id: string, props: PipelineStackProps) {
        super(scope, id, props);

        const { config } = props;

        // ── Lambda: resolve-commit ───────────────────────────────────────────
        // Input:  { "ref": "6.5.x" }
        // Output: { "ref": "6.5.x", "commitSha": "abc123..." }
        const resolveCommitFn = new lambda.Function(this, "ResolveCommitFn", {
            runtime: lambda.Runtime.PYTHON_3_12,
            handler: "index.handler",
            code: lambda.Code.fromAsset("pipeline/lambda/resolve_commit"),
            timeout: Duration.minutes(1),
            memorySize: 128,
            description: "Resolve Spring Security ref to HEAD commitSha via GitHub API",
        });

        // ── Lambda: cleanup ──────────────────────────────────────────────────
        // Input:  { "ref": "6.5.x", "commitSha": "abc123..." }
        // Output: { "chunks": N, "keywords": N, "vectors": N, "s3_objects": N }
        const cleanupFn = new lambda.Function(this, "CleanupFn", {
            runtime: lambda.Runtime.PYTHON_3_12,
            handler: "index.handler",
            code: lambda.Code.fromAsset("pipeline/lambda/cleanup"),
            timeout: Duration.minutes(15),
            memorySize: 256,
            description: "Remove stale commitSha data from DynamoDB, S3 Vectors, and S3",
            environment: {
                CHUNKS_TABLE: props.tables.chunks.tableName,
                KEYWORDS_TABLE: props.tables.keywords.tableName,
                CHUNKS_TABLE_GSI_NAME: props.chunksTableGsiName,
                VECTOR_INDEX: props.vectorIndex.ref,
                CONTENT_BUCKET: props.contentBucket.bucketName,
            },
        });
        props.tables.chunks.grantReadWriteData(cleanupFn);
        props.tables.keywords.grantReadWriteData(cleanupFn);
        props.contentBucket.grantReadWrite(cleanupFn);
        cleanupFn.addToRolePolicy(
            new iam.PolicyStatement({
                actions: ["s3vectors:DeleteVectors"],
                resources: [
                    props.vectorBucket.attrVectorBucketArn,
                    props.vectorIndex.ref,
                ],
            }),
        );

        // ── CodeBuild: Antora-only ───────────────────────────────────────────
        // Clones Spring Security, runs Antora build, uploads site.tar.gz to S3.
        // REF and COMMIT_SHA are overridden at runtime by Step Functions.
        if (config.githubTokenSecretName) {
            new codebuild.GitHubSourceCredentials(this, "GitHubCreds", {
                accessToken: SecretValue.secretsManager(
                    config.githubTokenSecretName,
                    { jsonField: "githubToken" },
                ),
            });
        }

        const buildRole = new iam.Role(this, "CodeBuildRole", {
            assumedBy: new iam.ServicePrincipal("codebuild.amazonaws.com"),
            description: "CodeBuild role for Antora build (clone + build + S3 upload)",
        });
        props.contentBucket.grantReadWrite(buildRole);

        const buildLogGroup = new logs.LogGroup(this, "BuildLogs", {
            retention: logs.RetentionDays.ONE_MONTH,
            removalPolicy: RemovalPolicy.DESTROY,
        });

        const buildProject = new codebuild.Project(this, "AntoraBuilder", {
            role: buildRole,
            source: codebuild.Source.gitHub({
                owner: "shutx-net",
                repo: "spring-security-documentation-mcp-server",
                webhook: false,
            }),
            environment: {
                buildImage: codebuild.LinuxBuildImage.STANDARD_7_0,
                computeType: codebuild.ComputeType.MEDIUM,
                privileged: false,
                environmentVariables: {
                    CONTENT_BUCKET: { value: props.contentBucket.bucketName },
                    REF: { value: "" },
                    COMMIT_SHA: { value: "" },
                },
            },
            buildSpec: codebuild.BuildSpec.fromSourceFilename("cdk/buildspec.yml"),
            cache: codebuild.Cache.bucket(props.contentBucket, {
                prefix: "codebuild-cache",
            }),
            timeout: Duration.hours(1),
            queuedTimeout: Duration.hours(1),
            logging: {
                cloudWatch: { logGroup: buildLogGroup, enabled: true },
            },
        });

        // ── ECS Fargate: indexer ─────────────────────────────────────────────
        // Reads site.tar.gz from S3, embeds via Bedrock, writes to DynamoDB + S3 Vectors.
        // Runs in public subnets with assignPublicIp because S3 Vectors has no VPC endpoint.
        const indexerCluster = new ecs.Cluster(this, "IndexerCluster", {
            vpc: props.vpc,
        });

        const indexerTaskRole = new iam.Role(this, "IndexerTaskRole", {
            assumedBy: new iam.ServicePrincipal("ecs-tasks.amazonaws.com"),
            description: "ECS indexer task role (S3, DynamoDB, Bedrock, S3 Vectors)",
        });
        props.contentBucket.grantReadWrite(indexerTaskRole);
        props.tables.chunks.grantReadWriteData(indexerTaskRole);
        props.tables.keywords.grantReadWriteData(indexerTaskRole);
        props.tables.embeddingCache.grantReadWriteData(indexerTaskRole);
        indexerTaskRole.addToPolicy(
            new iam.PolicyStatement({
                actions: ["bedrock:InvokeModel"],
                resources: [
                    `arn:aws:bedrock:${this.region}::foundation-model/${config.embeddingModelId}`,
                ],
            }),
        );
        indexerTaskRole.addToPolicy(
            new iam.PolicyStatement({
                actions: [
                    "s3vectors:PutVectors",
                    "s3vectors:DeleteVectors",
                    "s3vectors:ListVectors",
                    "s3vectors:GetVectors",
                ],
                resources: [
                    props.vectorBucket.attrVectorBucketArn,
                    props.vectorIndex.ref,
                ],
            }),
        );

        const indexerLogGroup = new logs.LogGroup(this, "IndexerLogs", {
            retention: logs.RetentionDays.ONE_MONTH,
            removalPolicy: RemovalPolicy.DESTROY,
        });

        const indexerTaskDef = new ecs.FargateTaskDefinition(this, "IndexerTaskDef", {
            cpu: 1024,
            memoryLimitMiB: 2048,
            taskRole: indexerTaskRole,
            runtimePlatform: {
                cpuArchitecture: ecs.CpuArchitecture.X86_64,
                operatingSystemFamily: ecs.OperatingSystemFamily.LINUX,
            },
        });

        // Use sh -c so $REF/$COMMIT_SHA injected via Step Functions env overrides expand at runtime.
        const indexerContainer = indexerTaskDef.addContainer("indexer", {
            image: ecs.ContainerImage.fromEcrRepository(props.indexerRepository, "latest"),
            logging: ecs.LogDrivers.awsLogs({
                streamPrefix: "indexer",
                logGroup: indexerLogGroup,
            }),
            environment: {
                CONTENT_BUCKET: props.contentBucket.bucketName,
                VECTOR_BUCKET: props.vectorBucket.ref,
                VECTOR_INDEX: props.vectorIndex.ref,
                CHUNKS_TABLE: props.tables.chunks.tableName,
                KEYWORDS_TABLE: props.tables.keywords.tableName,
                EMBEDDING_MODEL_ID: config.embeddingModelId,
                AWS_DEFAULT_REGION: this.region,
            },
            entryPoint: ["sh", "-c"],
            command: [
                'python indexer.py --artifact-s3-key "artifacts/$REF/$COMMIT_SHA/site.tar.gz" --ref "$REF" --commit-sha "$COMMIT_SHA"',
            ],
        });

        const indexerSg = new ec2.SecurityGroup(this, "IndexerSg", {
            vpc: props.vpc,
            description: "Indexer ECS task - egress only",
            allowAllOutbound: true,
        });

        // ── Step Functions task definitions ──────────────────────────────────

        // 1. resolve-commit: state → { ref, commitSha }
        const resolveCommitTask = new tasks.LambdaInvoke(this, "ResolveCommit", {
            lambdaFunction: resolveCommitFn,
            outputPath: "$.Payload",
            comment: "Fetch HEAD commitSha for the ref from GitHub API",
        });

        // 2. Antora build: produces artifacts/{ref}/{commitSha}/site.tar.gz in S3
        const antoraBuildTask = new tasks.CodeBuildStartBuild(this, "AntoraBuild", {
            project: buildProject,
            integrationPattern: sfn.IntegrationPattern.RUN_JOB,
            environmentVariablesOverride: {
                REF: {
                    type: codebuild.BuildEnvironmentVariableType.PLAINTEXT,
                    value: sfn.JsonPath.stringAt("$.ref"),
                },
                COMMIT_SHA: {
                    type: codebuild.BuildEnvironmentVariableType.PLAINTEXT,
                    value: sfn.JsonPath.stringAt("$.commitSha"),
                },
            },
            resultPath: sfn.JsonPath.DISCARD,
            comment: "Antora build → site.tar.gz → S3",
        });

        // 3. Indexer: embed + write DynamoDB + S3 Vectors
        const indexDocsTask = new tasks.EcsRunTask(this, "IndexDocs", {
            integrationPattern: sfn.IntegrationPattern.RUN_JOB,
            cluster: indexerCluster,
            taskDefinition: indexerTaskDef,
            assignPublicIp: true,
            subnets: { subnetType: ec2.SubnetType.PUBLIC },
            securityGroups: [indexerSg],
            launchTarget: new tasks.EcsFargateLaunchTarget({
                platformVersion: ecs.FargatePlatformVersion.LATEST,
            }),
            containerOverrides: [
                {
                    containerDefinition: indexerContainer,
                    environment: [
                        { name: "REF", value: sfn.JsonPath.stringAt("$.ref") },
                        { name: "COMMIT_SHA", value: sfn.JsonPath.stringAt("$.commitSha") },
                    ],
                },
            ],
            resultPath: sfn.JsonPath.DISCARD,
            comment: "Embed + write DynamoDB + S3 Vectors",
        });

        // 4. Cleanup: remove stale commitSha data from all stores
        const cleanupTask = new tasks.LambdaInvoke(this, "CleanupStale", {
            lambdaFunction: cleanupFn,
            payload: sfn.TaskInput.fromObject({
                ref: sfn.JsonPath.stringAt("$.ref"),
                commitSha: sfn.JsonPath.stringAt("$.commitSha"),
            }),
            resultPath: sfn.JsonPath.DISCARD,
            comment: "Delete stale data from DynamoDB, S3 Vectors, S3",
        });

        // ── State machine ────────────────────────────────────────────────────
        const processRef = resolveCommitTask
            .next(antoraBuildTask)
            .next(indexDocsTask)
            .next(cleanupTask);

        const processRefs = new sfn.Map(this, "ProcessRefs", {
            maxConcurrency: 2,
            itemsPath: sfn.JsonPath.stringAt("$.refs"),
            comment: "Process each Spring Security ref in parallel",
        }).itemProcessor(processRef);

        const smLogGroup = new logs.LogGroup(this, "StateMachineLogs", {
            retention: logs.RetentionDays.ONE_MONTH,
            removalPolicy: RemovalPolicy.DESTROY,
        });

        this.stateMachine = new sfn.StateMachine(this, "IndexPipeline", {
            definitionBody: sfn.DefinitionBody.fromChainable(processRefs),
            stateMachineType: sfn.StateMachineType.STANDARD,
            timeout: Duration.hours(3),
            logs: {
                destination: smLogGroup,
                level: sfn.LogLevel.ERROR,
            },
        });

        // ── EventBridge Scheduler → State Machine ────────────────────────────
        const schedulerRole = new iam.Role(this, "SchedulerRole", {
            assumedBy: new iam.ServicePrincipal("scheduler.amazonaws.com"),
        });
        this.stateMachine.grantStartExecution(schedulerRole);

        new scheduler.CfnSchedule(this, "DailyIndexSchedule", {
            flexibleTimeWindow: { mode: "OFF" },
            scheduleExpression: config.schedule,
            scheduleExpressionTimezone: config.scheduleTimezone,
            state: "ENABLED",
            target: {
                arn: this.stateMachine.stateMachineArn,
                roleArn: schedulerRole.roleArn,
                input: JSON.stringify({
                    refs: [{ ref: "6.5.x" }, { ref: "7.0.x" }],
                }),
                retryPolicy: { maximumRetryAttempts: 0 },
            },
            description: "Daily Spring Security docs rebuild via Step Functions",
        });

        new CfnOutput(this, "StateMachineArn", {
            value: this.stateMachine.stateMachineArn,
        });
        new CfnOutput(this, "IndexerClusterName", {
            value: indexerCluster.clusterName,
        });
    }
}
