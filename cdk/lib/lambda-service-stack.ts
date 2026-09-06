import * as path from 'path';
import {
  Stack,
  StackProps,
  Duration,
  RemovalPolicy,
  CfnOutput,
  aws_lambda as lambda,
  aws_apigatewayv2 as apigwv2,
  aws_apigatewayv2_integrations as apigwv2Integrations,
  aws_iam as iam,
  aws_logs as logs,
  aws_certificatemanager as acm,
  aws_s3vectors as s3vectors,
  aws_s3 as s3,
} from 'aws-cdk-lib';
import { Construct } from 'constructs';
import { AppConfig } from './config';
import { DocTables } from './storage-stack';

export interface LambdaServiceStackProps extends StackProps {
  readonly config: AppConfig;
  readonly contentBucket: s3.IBucket;
  readonly vectorBucket: s3vectors.CfnVectorBucket;
  readonly vectorIndex: s3vectors.CfnIndex;
  readonly tables: DocTables;
}

export class LambdaServiceStack extends Stack {
  public readonly functionArn: string;
  public readonly functionName: string;

  constructor(scope: Construct, id: string, props: LambdaServiceStackProps) {
    super(scope, id, props);

    const { config } = props;

    const logGroup = new logs.LogGroup(this, 'AppLogs', {
      retention: logs.RetentionDays.ONE_MONTH,
      removalPolicy: RemovalPolicy.DESTROY,
    });

    const fn = new lambda.Function(this, 'McpFn', {
      runtime: lambda.Runtime.PROVIDED_AL2023,
      architecture: lambda.Architecture.ARM_64,
      handler: 'bootstrap',
      // Placeholder binary; the real one is pushed by CI via update-function-code.
      code: lambda.Code.fromAsset(path.join(__dirname, '..', 'lambda', 'mcp-placeholder')),
      memorySize: config.lambda.memoryMiB,
      timeout: Duration.seconds(config.lambda.timeoutSeconds),
      reservedConcurrentExecutions: config.lambda.reservedConcurrency,
      logGroup,
      environment: {
        APP_NAME: config.appName,
        CONTENT_BUCKET: props.contentBucket.bucketName,
        VECTOR_BUCKET: props.vectorBucket.ref,
        VECTOR_INDEX: props.vectorIndex.ref,
        CHUNKS_TABLE: props.tables.chunks.tableName,
        KEYWORDS_TABLE: props.tables.keywords.tableName,
        EMBEDDING_CACHE_TABLE: props.tables.embeddingCache.tableName,
        RATE_LIMITS_TABLE: props.tables.rateLimits.tableName,
        EMBEDDING_MODEL_ID: config.embeddingModelId,
      },
    });

    props.contentBucket.grantRead(fn);
    props.tables.chunks.grantReadData(fn);
    props.tables.keywords.grantReadData(fn);
    props.tables.embeddingCache.grantReadWriteData(fn);
    props.tables.rateLimits.grantReadWriteData(fn);
    fn.addToRolePolicy(
      new iam.PolicyStatement({
        actions: ['bedrock:InvokeModel'],
        resources: [
          `arn:aws:bedrock:${this.region}::foundation-model/${config.embeddingModelId}`,
        ],
      }),
    );
    fn.addToRolePolicy(
      new iam.PolicyStatement({
        actions: ['s3vectors:QueryVectors', 's3vectors:GetVectors'],
        resources: [
          props.vectorBucket.attrVectorBucketArn,
          props.vectorIndex.ref,
        ],
      }),
    );

    if (!config.domain) {
      throw new Error(
        'ACM certificate is required for the API custom domain. Set context "domainName" and "certificateArn".',
      );
    }

    const certificate = acm.Certificate.fromCertificateArn(
      this,
      'ApiCertificate',
      config.domain.certificateArn,
    );

    const domain = new apigwv2.DomainName(this, 'Domain', {
      domainName: config.domain.domainName,
      certificate,
    });

    const httpApi = new apigwv2.HttpApi(this, 'HttpApi', {
      defaultIntegration: new apigwv2Integrations.HttpLambdaIntegration('McpIntegration', fn),
      createDefaultStage: false,
      // HTTP APIs cannot restrict callers by IP, so the execute-api endpoint would
      // be a way to reach the server without passing through Cloudflare's WAF and
      // rate limits. Disabling it leaves the custom domain as the only entry point.
      disableExecuteApiEndpoint: true,
    });

    new apigwv2.HttpStage(this, 'DefaultStage', {
      httpApi,
      autoDeploy: true,
      throttle: {
        rateLimit: config.lambda.throttleRateLimit,
        burstLimit: config.lambda.throttleBurstLimit,
      },
      domainMapping: { domainName: domain },
    });

    this.functionArn = fn.functionArn;
    this.functionName = fn.functionName;

    new CfnOutput(this, 'McpFunctionName', { value: fn.functionName });
    new CfnOutput(this, 'PublicUrl', { value: `https://${config.domain.domainName}/mcp` });
    new CfnOutput(this, 'CloudflareOriginTarget', {
      value: domain.regionalDomainName,
      description: 'Repoint the Cloudflare CNAME to this DNS name',
    });
  }
}
