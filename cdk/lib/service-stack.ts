import {
  Stack,
  StackProps,
  Duration,
  RemovalPolicy,
  CfnOutput,
  Fn,
  aws_ec2 as ec2,
  aws_ecs as ecs,
  aws_ecr as ecr,
  aws_elasticloadbalancingv2 as elbv2,
  aws_iam as iam,
  aws_logs as logs,
  aws_cloudfront as cloudfront,
  aws_cloudfront_origins as origins,
  aws_certificatemanager as acm,
  aws_route53 as route53,
  aws_route53_targets as targets,
  aws_wafv2 as wafv2,
  aws_s3vectors as s3vectors,
  aws_s3 as s3,
} from 'aws-cdk-lib';
import { Construct } from 'constructs';
import { AppConfig } from './config';
import { DocTables } from './storage-stack';

export interface ServiceStackProps extends StackProps {
  readonly config: AppConfig;
  readonly vpc: ec2.IVpc;
  readonly albSecurityGroup: ec2.SecurityGroup;
  readonly ecsSecurityGroup: ec2.SecurityGroup;
  readonly contentBucket: s3.IBucket;
  readonly vectorBucket: s3vectors.CfnVectorBucket;
  readonly vectorIndex: s3vectors.CfnIndex;
  readonly tables: DocTables;
}

export class ServiceStack extends Stack {
  public readonly ecrRepository: ecr.IRepository;
  public readonly distribution: cloudfront.IDistribution;

  constructor(scope: Construct, id: string, props: ServiceStackProps) {
    super(scope, id, props);

    const { config, vpc, albSecurityGroup, ecsSecurityGroup } = props;

    this.ecrRepository = new ecr.Repository(this, 'AppImageRepo', {
      imageScanOnPush: true,
      encryption: ecr.RepositoryEncryption.AES_256,
      removalPolicy: RemovalPolicy.RETAIN,
      lifecycleRules: [
        { maxImageCount: 10, description: 'Retain the 10 most recent images' },
      ],
    });

    const cluster = new ecs.Cluster(this, 'Cluster', {
      vpc,
      containerInsightsV2: ecs.ContainerInsights.ENABLED,
    });

    const taskRole = new iam.Role(this, 'TaskRole', {
      assumedBy: new iam.ServicePrincipal('ecs-tasks.amazonaws.com'),
      description: 'ECS task role for MCP server (read storage, invoke Bedrock embed)',
    });
    props.contentBucket.grantRead(taskRole);
    props.tables.chunks.grantReadData(taskRole);
    props.tables.keywords.grantReadData(taskRole);
    props.tables.embeddingCache.grantReadWriteData(taskRole);
    props.tables.rateLimits.grantReadWriteData(taskRole);
    taskRole.addToPolicy(
      new iam.PolicyStatement({
        actions: ['bedrock:InvokeModel'],
        resources: [
          `arn:aws:bedrock:${this.region}::foundation-model/${config.embeddingModelId}`,
        ],
      }),
    );
    taskRole.addToPolicy(
      new iam.PolicyStatement({
        actions: ['s3vectors:QueryVectors', 's3vectors:GetVectors'],
        resources: [
          props.vectorBucket.attrVectorBucketArn,
          `${props.vectorBucket.attrVectorBucketArn}/index/${props.vectorIndex.ref}`,
        ],
      }),
    );

    const logGroup = new logs.LogGroup(this, 'AppLogs', {
      retention: logs.RetentionDays.ONE_MONTH,
      removalPolicy: RemovalPolicy.DESTROY,
    });

    const taskDef = new ecs.FargateTaskDefinition(this, 'TaskDef', {
      cpu: config.ecs.cpu,
      memoryLimitMiB: config.ecs.memoryMiB,
      runtimePlatform: {
        cpuArchitecture: ecs.CpuArchitecture.X86_64,
        operatingSystemFamily: ecs.OperatingSystemFamily.LINUX,
      },
      taskRole,
    });

    taskDef.addContainer('app', {
      image: ecs.ContainerImage.fromEcrRepository(this.ecrRepository, 'latest'),
      logging: ecs.LogDrivers.awsLogs({ streamPrefix: 'mcp', logGroup }),
      portMappings: [{ containerPort: config.ecs.containerPort, protocol: ecs.Protocol.TCP }],
      environment: {
        AWS_REGION: this.region,
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

    const privateAzA = vpc.selectSubnets({
      subnetType: ec2.SubnetType.PRIVATE_ISOLATED,
      availabilityZones: [vpc.availabilityZones[0]],
    });

    const service = new ecs.FargateService(this, 'Service', {
      cluster,
      taskDefinition: taskDef,
      desiredCount: config.ecs.desiredCount,
      assignPublicIp: false,
      vpcSubnets: privateAzA,
      securityGroups: [ecsSecurityGroup],
      circuitBreaker: { rollback: true },
      minHealthyPercent: 100,
      maxHealthyPercent: 200,
      enableExecuteCommand: false,
    });

    const scalable = service.autoScaleTaskCount({
      minCapacity: config.ecs.minCapacity,
      maxCapacity: config.ecs.maxCapacity,
    });

    const alb = new elbv2.ApplicationLoadBalancer(this, 'Alb', {
      vpc,
      internetFacing: true,
      securityGroup: albSecurityGroup,
      deletionProtection: false,
    });

    const listener = alb.addListener('HttpListener', {
      port: 80,
      protocol: elbv2.ApplicationProtocol.HTTP,
      open: false,
    });

    const targetGroup = listener.addTargets('EcsTarget', {
      port: config.ecs.containerPort,
      protocol: elbv2.ApplicationProtocol.HTTP,
      targets: [service],
      healthCheck: {
        path: '/healthz',
        interval: Duration.seconds(30),
        timeout: Duration.seconds(5),
        healthyThresholdCount: 2,
        unhealthyThresholdCount: 3,
        healthyHttpCodes: '200',
      },
      deregistrationDelay: Duration.seconds(30),
    });

    scalable.scaleOnRequestCount('RequestCountScaling', {
      requestsPerTarget: config.ecs.requestsPerTarget,
      targetGroup,
      scaleInCooldown: Duration.seconds(300),
      scaleOutCooldown: Duration.seconds(60),
    });

    const webAcl = this.buildWebAcl(config);

    const cachePolicy = new cloudfront.CachePolicy(this, 'McpCachePolicy', {
      defaultTtl: Duration.seconds(0),
      maxTtl: Duration.seconds(0),
      minTtl: Duration.seconds(0),
      headerBehavior: cloudfront.CacheHeaderBehavior.allowList('Authorization', 'Content-Type'),
      cookieBehavior: cloudfront.CacheCookieBehavior.none(),
      queryStringBehavior: cloudfront.CacheQueryStringBehavior.all(),
      enableAcceptEncodingGzip: true,
      enableAcceptEncodingBrotli: true,
    });

    const originRequestPolicy = new cloudfront.OriginRequestPolicy(this, 'McpOriginRequestPolicy', {
      headerBehavior: cloudfront.OriginRequestHeaderBehavior.allowList(
        'Content-Type',
        'Accept',
        'User-Agent',
      ),
      cookieBehavior: cloudfront.OriginRequestCookieBehavior.none(),
      queryStringBehavior: cloudfront.OriginRequestQueryStringBehavior.all(),
    });

    const albOrigin = new origins.LoadBalancerV2Origin(alb, {
      protocolPolicy: cloudfront.OriginProtocolPolicy.HTTP_ONLY,
      httpPort: 80,
      readTimeout: Duration.seconds(30),
      keepaliveTimeout: Duration.seconds(5),
    });

    const distribution = new cloudfront.Distribution(this, 'Distribution', {
      defaultBehavior: {
        origin: albOrigin,
        viewerProtocolPolicy: cloudfront.ViewerProtocolPolicy.REDIRECT_TO_HTTPS,
        allowedMethods: cloudfront.AllowedMethods.ALLOW_ALL,
        cachePolicy,
        originRequestPolicy,
        compress: true,
      },
      webAclId: webAcl.attrArn,
      priceClass: cloudfront.PriceClass.PRICE_CLASS_200,
      minimumProtocolVersion: cloudfront.SecurityPolicyProtocol.TLS_V1_2_2021,
      httpVersion: cloudfront.HttpVersion.HTTP2_AND_3,
      ...(config.domain
        ? {
            domainNames: [config.domain.domainName],
            certificate: acm.Certificate.fromCertificateArn(
              this,
              'CloudFrontCert',
              config.domain.certificateArn,
            ),
          }
        : {}),
    });
    this.distribution = distribution;

    if (config.domain) {
      const zone = route53.HostedZone.fromHostedZoneAttributes(this, 'Zone', {
        hostedZoneId: config.domain.hostedZoneId,
        zoneName: config.domain.hostedZoneName,
      });
      new route53.ARecord(this, 'AliasRecord', {
        zone,
        recordName: Fn.select(0, Fn.split(`.${config.domain.hostedZoneName}`, config.domain.domainName)),
        target: route53.RecordTarget.fromAlias(new targets.CloudFrontTarget(distribution)),
      });
    }

    new CfnOutput(this, 'EcrRepositoryUri', { value: this.ecrRepository.repositoryUri });
    new CfnOutput(this, 'AlbDnsName', { value: alb.loadBalancerDnsName });
    new CfnOutput(this, 'DistributionDomainName', { value: distribution.distributionDomainName });
    if (config.domain) {
      new CfnOutput(this, 'PublicUrl', { value: `https://${config.domain.domainName}` });
    }
  }

  private buildWebAcl(config: AppConfig): wafv2.CfnWebACL {
    const rules: wafv2.CfnWebACL.RuleProperty[] = [
      {
        name: 'AWS-AWSManagedRulesCommonRuleSet',
        priority: 0,
        overrideAction: { none: {} },
        statement: {
          managedRuleGroupStatement: {
            vendorName: 'AWS',
            name: 'AWSManagedRulesCommonRuleSet',
          },
        },
        visibilityConfig: {
          cloudWatchMetricsEnabled: true,
          metricName: 'CommonRuleSet',
          sampledRequestsEnabled: true,
        },
      },
      {
        name: 'AWS-AWSManagedRulesKnownBadInputsRuleSet',
        priority: 1,
        overrideAction: { none: {} },
        statement: {
          managedRuleGroupStatement: {
            vendorName: 'AWS',
            name: 'AWSManagedRulesKnownBadInputsRuleSet',
          },
        },
        visibilityConfig: {
          cloudWatchMetricsEnabled: true,
          metricName: 'KnownBadInputs',
          sampledRequestsEnabled: true,
        },
      },
      {
        name: 'IpRateLimit',
        priority: 10,
        action: { block: {} },
        statement: {
          rateBasedStatement: {
            limit: config.waf.rateLimitPer5Min,
            aggregateKeyType: 'IP',
          },
        },
        visibilityConfig: {
          cloudWatchMetricsEnabled: true,
          metricName: 'IpRateLimit',
          sampledRequestsEnabled: true,
        },
      },
      {
        name: 'BodySizeLimit',
        priority: 11,
        action: { block: {} },
        statement: {
          sizeConstraintStatement: {
            fieldToMatch: { body: {} },
            comparisonOperator: 'GT',
            size: config.waf.maxBodyBytes,
            textTransformations: [{ priority: 0, type: 'NONE' }],
          },
        },
        visibilityConfig: {
          cloudWatchMetricsEnabled: true,
          metricName: 'BodySizeLimit',
          sampledRequestsEnabled: true,
        },
      },
    ];

    return new wafv2.CfnWebACL(this, 'WebAcl', {
      defaultAction: { allow: {} },
      scope: 'CLOUDFRONT',
      visibilityConfig: {
        cloudWatchMetricsEnabled: true,
        metricName: `${config.appName}-webacl`,
        sampledRequestsEnabled: true,
      },
      rules,
    });
  }
}
