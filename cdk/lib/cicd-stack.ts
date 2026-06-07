import {
  Stack,
  StackProps,
  RemovalPolicy,
  CfnOutput,
  aws_iam as iam,
  aws_ecr as ecr,
  aws_s3 as s3,
  aws_s3vectors as s3vectors,
} from 'aws-cdk-lib';
import { Construct } from 'constructs';
import { AppConfig } from './config';
import { DocTables } from './storage-stack';

export interface CicdStackProps extends StackProps {
  readonly githubOrg: string;
  readonly githubRepo: string;
  readonly ecrRepository: ecr.IRepository;
  readonly ecsServiceArn: string;
  readonly indexerRepository: ecr.IRepository;
  readonly config: AppConfig;
  readonly vectorBucket: s3vectors.CfnVectorBucket;
  readonly vectorIndex: s3vectors.CfnIndex;
  readonly tables: DocTables;
}

export class CicdStack extends Stack {
  constructor(scope: Construct, id: string, props: CicdStackProps) {
    super(scope, id, props);

    const {
      githubOrg,
      githubRepo,
      ecrRepository,
      ecsServiceArn,
      indexerRepository,
      config,
      vectorBucket,
      vectorIndex,
      tables,
    } = props;

    // OIDC provider for GitHub Actions.
    // OidcProviderNative uses the native AWS::IAM::OIDCProvider CloudFormation resource
    // (recommended over OpenIdConnectProvider which relies on a custom resource).
    // Thumbprints: GitHub publishes two valid thumbprints for token.actions.githubusercontent.com.
    const oidcProvider = new iam.OidcProviderNative(this, 'GitHubOidcProvider', {
      url: 'https://token.actions.githubusercontent.com',
      clientIds: ['sts.amazonaws.com'],
      thumbprints: [
        '6938fd4d98bab03faadb97b34396831e3780aea1',
        '1b511abead59c6ce207077c0bf0e0043b1382612',
      ],
    });

    const githubPrincipal = new iam.WebIdentityPrincipal(
      oidcProvider.openIdConnectProviderArn,
      {
        StringEquals: {
          'token.actions.githubusercontent.com:aud': 'sts.amazonaws.com',
        },
        StringLike: {
          // Allow any branch/tag/PR from this repository.
          'token.actions.githubusercontent.com:sub': `repo:${githubOrg}/${githubRepo}:*`,
        },
      },
    );

    const role = new iam.Role(this, 'GitHubActionsEcrPushRole', {
      assumedBy: githubPrincipal,
      description: 'Assumed by GitHub Actions to push images to ECR',
    });

    // ECR push permissions (docker buildx requires BatchGetImage for manifest HEAD checks).
    role.addToPolicy(new iam.PolicyStatement({
      actions: [
        'ecr:BatchCheckLayerAvailability',
        'ecr:BatchGetImage',
        'ecr:CompleteLayerUpload',
        'ecr:GetDownloadUrlForLayer',
        'ecr:InitiateLayerUpload',
        'ecr:PutImage',
        'ecr:UploadLayerPart',
      ],
      resources: [ecrRepository.repositoryArn, indexerRepository.repositoryArn],
    }));

    // GetAuthorizationToken operates on * (not a specific repository).
    role.addToPolicy(new iam.PolicyStatement({
      actions: ['ecr:GetAuthorizationToken'],
      resources: ['*'],
    }));

    // ECS force-new-deployment after ECR push.
    role.addToPolicy(new iam.PolicyStatement({
      actions: ['ecs:UpdateService'],
      resources: [ecsServiceArn],
    }));

    new CfnOutput(this, 'GitHubActionsRoleArn', {
      value: role.roleArn,
      description: 'Set as AWS_ROLE_ARN secret in GitHub repository settings',
    });

    // Dedicated bucket for eval inputs (topics/qrels) and outputs (run/summary),
    // kept separate from contentBucket so the eval role never needs access to indexed content.
    const evalBucket = new s3.Bucket(this, 'EvalBucket', {
      encryption: s3.BucketEncryption.S3_MANAGED,
      blockPublicAccess: s3.BlockPublicAccess.BLOCK_ALL,
      enforceSSL: true,
      removalPolicy: RemovalPolicy.RETAIN,
    });

    const evalRole = new iam.Role(this, 'GitHubActionsMcpEvalRole', {
      assumedBy: githubPrincipal,
      description: 'Assumed by GitHub Actions to run eval run/eval score against the live index',
    });

    // Read eval inputs (topics.jsonl/qrels.jsonl) and write eval outputs (run.jsonl/summary.json).
    evalRole.addToPolicy(new iam.PolicyStatement({
      actions: ['s3:GetObject', 's3:PutObject'],
      resources: [evalBucket.arnForObjects('*')],
    }));

    // Read access for the search backend (chunks/keywords) plus read+write for the embedding cache.
    evalRole.addToPolicy(new iam.PolicyStatement({
      actions: ['dynamodb:GetItem', 'dynamodb:BatchGetItem', 'dynamodb:Query'],
      resources: [
        tables.chunks.tableArn,
        `${tables.chunks.tableArn}/index/*`,
        tables.keywords.tableArn,
        tables.embeddingCache.tableArn,
      ],
    }));
    // Search() unconditionally runs keywordSearch, which Scans the chunks table
    // with a contains() filter (internal/store/aws.go) — required even when
    // hybrid/vector search is also configured.
    evalRole.addToPolicy(new iam.PolicyStatement({
      actions: ['dynamodb:Scan'],
      resources: [tables.chunks.tableArn],
    }));
    evalRole.addToPolicy(new iam.PolicyStatement({
      actions: ['dynamodb:PutItem'],
      resources: [tables.embeddingCache.tableArn],
    }));

    evalRole.addToPolicy(new iam.PolicyStatement({
      actions: ['bedrock:InvokeModel'],
      resources: [
        `arn:aws:bedrock:${this.region}::foundation-model/${config.embeddingModelId}`,
      ],
    }));

    evalRole.addToPolicy(new iam.PolicyStatement({
      actions: ['s3vectors:QueryVectors'],
      resources: [
        vectorBucket.attrVectorBucketArn,
        vectorIndex.ref,
      ],
    }));

    new CfnOutput(this, 'GitHubActionsMcpEvalRoleArn', {
      value: evalRole.roleArn,
      description: 'Set as AWS_EVAL_ROLE_ARN secret for the eval workflow in GitHub repository settings',
    });
    new CfnOutput(this, 'EvalBucketName', {
      value: evalBucket.bucketName,
      description: 'Bucket for eval inputs (topics/qrels) and outputs (run/summary)',
    });
  }
}
