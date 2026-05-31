import {
    Stack,
    StackProps,
    Duration,
    RemovalPolicy,
    SecretValue,
    CfnOutput,
    aws_codebuild as codebuild,
    aws_iam as iam,
    aws_logs as logs,
    aws_s3 as s3,
    aws_scheduler as scheduler,
    aws_s3vectors as s3vectors,
} from "aws-cdk-lib";
import { Construct } from "constructs";
import { AppConfig } from "./config";
import { DocTables } from "./storage-stack";

export interface PipelineStackProps extends StackProps {
    readonly config: AppConfig;
    readonly contentBucket: s3.IBucket;
    readonly vectorBucket: s3vectors.CfnVectorBucket;
    readonly vectorIndex: s3vectors.CfnIndex;
    readonly tables: DocTables;
}

export class PipelineStack extends Stack {
    public readonly buildProject: codebuild.Project;

    constructor(scope: Construct, id: string, props: PipelineStackProps) {
        super(scope, id, props);

        const { config } = props;

        if (config.githubTokenSecretName) {
            new codebuild.GitHubSourceCredentials(this, 'GitHubCreds', {
                accessToken: SecretValue.secretsManager(config.githubTokenSecretName, { jsonField: 'githubToken' }),
            });
        }

        const buildRole = new iam.Role(this, "CodeBuildRole", {
            assumedBy: new iam.ServicePrincipal("codebuild.amazonaws.com"),
            description:
                "CodeBuild role for Spring Security docs indexing pipeline",
        });

        props.contentBucket.grantReadWrite(buildRole);
        props.tables.chunks.grantReadWriteData(buildRole);
        props.tables.keywords.grantReadWriteData(buildRole);
        props.tables.embeddingCache.grantReadWriteData(buildRole);

        buildRole.addToPolicy(
            new iam.PolicyStatement({
                actions: ["bedrock:InvokeModel"],
                resources: [
                    `arn:aws:bedrock:${this.region}::foundation-model/${config.embeddingModelId}`,
                ],
            }),
        );

        buildRole.addToPolicy(
            new iam.PolicyStatement({
                actions: [
                    "s3vectors:PutVectors",
                    "s3vectors:DeleteVectors",
                    "s3vectors:ListVectors",
                    "s3vectors:GetVectors",
                    "s3vectors:QueryVectors",
                ],
                resources: [
                    props.vectorBucket.attrVectorBucketArn,
                    props.vectorIndex.ref,
                ],
            }),
        );

        const buildLogGroup = new logs.LogGroup(this, "BuildLogs", {
            retention: logs.RetentionDays.ONE_MONTH,
            removalPolicy: RemovalPolicy.DESTROY,
        });

        this.buildProject = new codebuild.Project(this, "IndexBuild", {
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
                    VECTOR_BUCKET: { value: props.vectorBucket.ref },
                    VECTOR_INDEX: { value: props.vectorIndex.ref },
                    CHUNKS_TABLE: { value: props.tables.chunks.tableName },
                    KEYWORDS_TABLE: { value: props.tables.keywords.tableName },
                    EMBEDDING_MODEL_ID: { value: config.embeddingModelId },
                },
            },
            buildSpec:
                codebuild.BuildSpec.fromSourceFilename("cdk/buildspec.yml"),
            cache: codebuild.Cache.bucket(props.contentBucket, {
                prefix: "codebuild-cache",
            }),
            timeout: Duration.hours(1),
            queuedTimeout: Duration.hours(1),
            logging: {
                cloudWatch: { logGroup: buildLogGroup, enabled: true },
            },
        });

        const schedulerRole = new iam.Role(this, "SchedulerRole", {
            assumedBy: new iam.ServicePrincipal("scheduler.amazonaws.com"),
        });
        schedulerRole.addToPolicy(
            new iam.PolicyStatement({
                actions: ["codebuild:StartBuild"],
                resources: [this.buildProject.projectArn],
            }),
        );

        new scheduler.CfnSchedule(this, "DailyIndexSchedule", {
            flexibleTimeWindow: { mode: "OFF" },
            scheduleExpression: config.schedule,
            scheduleExpressionTimezone: config.scheduleTimezone,
            state: "ENABLED",
            target: {
                arn: this.buildProject.projectArn,
                roleArn: schedulerRole.roleArn,
                retryPolicy: { maximumRetryAttempts: 2 },
            },
            description: "Daily Spring Security docs index rebuild",
        });

        new CfnOutput(this, "BuildProjectName", {
            value: this.buildProject.projectName,
        });
    }
}
