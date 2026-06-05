import {
  Stack,
  StackProps,
  RemovalPolicy,
  Duration,
  CfnOutput,
  aws_dynamodb as dynamodb,
  aws_s3 as s3,
  aws_s3vectors as s3vectors,
  aws_ssm as ssm,
} from 'aws-cdk-lib';
import { Construct } from 'constructs';
import { AppConfig } from './config';

export interface StorageStackProps extends StackProps {
  readonly config: AppConfig;
}

export interface DocTables {
  readonly chunks: dynamodb.ITable;
  readonly keywords: dynamodb.ITable;
  readonly embeddingCache: dynamodb.ITable;
  readonly rateLimits: dynamodb.ITable;
}

export class StorageStack extends Stack {
  public readonly contentBucket: s3.IBucket;
  public readonly vectorBucket: s3vectors.CfnVectorBucket;
  public readonly vectorIndex: s3vectors.CfnIndex;
  public readonly tables: DocTables;
  public readonly chunksTableGsiName = 'ref-commitSha-index';

  constructor(scope: Construct, id: string, props: StorageStackProps) {
    super(scope, id, { ...props, terminationProtection: true });

    const { config } = props;

    this.contentBucket = new s3.Bucket(this, 'ContentBucket', {
      encryption: s3.BucketEncryption.S3_MANAGED,
      blockPublicAccess: s3.BlockPublicAccess.BLOCK_ALL,
      enforceSSL: true,
      versioned: true,
      removalPolicy: RemovalPolicy.RETAIN,
      lifecycleRules: [
        {
          noncurrentVersionExpiration: Duration.days(90),
        },
      ],
    });

    const vectorBucketName = `${config.appName}-vectors`;

    this.vectorBucket = new s3vectors.CfnVectorBucket(this, 'VectorBucket', {
      vectorBucketName,
      encryptionConfiguration: {
        sseType: 'AES256',
      },
    });

    this.vectorIndex = new s3vectors.CfnIndex(this, 'VectorIndex', {
      vectorBucketName,
      dataType: 'float32',
      dimension: config.embeddingDimension,
      distanceMetric: 'cosine',
      metadataConfiguration: {
        nonFilterableMetadataKeys: ['contentPreview', 'sourcePath'],
      },
    });
    this.vectorIndex.addDependency(this.vectorBucket);

    const chunks = new dynamodb.Table(this, 'DocChunksTable', {
      partitionKey: { name: 'chunkId', type: dynamodb.AttributeType.STRING },
      billingMode: dynamodb.BillingMode.PAY_PER_REQUEST,
      encryption: dynamodb.TableEncryption.AWS_MANAGED,
      removalPolicy: RemovalPolicy.RETAIN,
      pointInTimeRecoverySpecification: { pointInTimeRecoveryEnabled: true },
    });

    chunks.addGlobalSecondaryIndex({
      indexName: this.chunksTableGsiName,
      partitionKey: { name: 'ref', type: dynamodb.AttributeType.STRING },
      sortKey: { name: 'commitSha', type: dynamodb.AttributeType.STRING },
      projectionType: dynamodb.ProjectionType.KEYS_ONLY,
    });

    const keywords = new dynamodb.Table(this, 'DocKeywordsTable', {
      partitionKey: { name: 'keyword', type: dynamodb.AttributeType.STRING },
      sortKey: { name: 'refAreaChunkId', type: dynamodb.AttributeType.STRING },
      billingMode: dynamodb.BillingMode.PAY_PER_REQUEST,
      encryption: dynamodb.TableEncryption.AWS_MANAGED,
      removalPolicy: RemovalPolicy.RETAIN,
      pointInTimeRecoverySpecification: { pointInTimeRecoveryEnabled: true },
    });

    const embeddingCache = new dynamodb.Table(this, 'EmbeddingCacheTable', {
      partitionKey: { name: 'queryHash', type: dynamodb.AttributeType.STRING },
      billingMode: dynamodb.BillingMode.PAY_PER_REQUEST,
      encryption: dynamodb.TableEncryption.AWS_MANAGED,
      timeToLiveAttribute: 'ttl',
      removalPolicy: RemovalPolicy.DESTROY,
    });

    const rateLimits = new dynamodb.Table(this, 'RateLimitsTable', {
      partitionKey: { name: 'bucket', type: dynamodb.AttributeType.STRING },
      billingMode: dynamodb.BillingMode.PAY_PER_REQUEST,
      encryption: dynamodb.TableEncryption.AWS_MANAGED,
      timeToLiveAttribute: 'ttl',
      removalPolicy: RemovalPolicy.DESTROY,
    });

    this.tables = { chunks, keywords, embeddingCache, rateLimits };

    const paramPrefix = `/${config.appName}`;
    new ssm.StringParameter(this, 'ParamContentBucket', {
      parameterName: `${paramPrefix}/content-bucket-name`,
      stringValue: this.contentBucket.bucketName,
    });
    new ssm.StringParameter(this, 'ParamVectorBucket', {
      parameterName: `${paramPrefix}/vector-bucket-name`,
      stringValue: this.vectorBucket.ref,
    });
    new ssm.StringParameter(this, 'ParamVectorIndex', {
      parameterName: `${paramPrefix}/vector-index-name`,
      stringValue: this.vectorIndex.ref,
    });
    new ssm.StringParameter(this, 'ParamChunksTable', {
      parameterName: `${paramPrefix}/dynamodb/chunks-table-name`,
      stringValue: chunks.tableName,
    });
    new ssm.StringParameter(this, 'ParamChunksTableGsiName', {
      parameterName: `${paramPrefix}/dynamodb/chunks-table-gsi-name`,
      stringValue: this.chunksTableGsiName,
    });
    new ssm.StringParameter(this, 'ParamKeywordsTable', {
      parameterName: `${paramPrefix}/dynamodb/keywords-table-name`,
      stringValue: keywords.tableName,
    });
    new ssm.StringParameter(this, 'ParamEmbeddingCacheTable', {
      parameterName: `${paramPrefix}/dynamodb/embedding-cache-table-name`,
      stringValue: embeddingCache.tableName,
    });
    new ssm.StringParameter(this, 'ParamRateLimitsTable', {
      parameterName: `${paramPrefix}/dynamodb/rate-limits-table-name`,
      stringValue: rateLimits.tableName,
    });

    new CfnOutput(this, 'ContentBucketName', { value: this.contentBucket.bucketName });
    new CfnOutput(this, 'VectorBucketName', { value: this.vectorBucket.ref });
    new CfnOutput(this, 'VectorIndexName', { value: this.vectorIndex.ref });
  }
}
