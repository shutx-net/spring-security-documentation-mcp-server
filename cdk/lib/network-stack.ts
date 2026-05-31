import * as path from 'path';
import {
  Stack,
  StackProps,
  Duration,
  RemovalPolicy,
  aws_ec2 as ec2,
  aws_lambda as lambda,
  aws_events as events,
  aws_events_targets as eventTargets,
  aws_iam as iam,
  aws_logs as logs,
} from 'aws-cdk-lib';
import { Construct } from 'constructs';
import { AppConfig } from './config';

export interface NetworkStackProps extends StackProps {
  readonly config: AppConfig;
}

export class NetworkStack extends Stack {
  public readonly vpc: ec2.IVpc;
  public readonly albSecurityGroup: ec2.SecurityGroup;
  public readonly ecsSecurityGroup: ec2.SecurityGroup;
  public readonly endpointSecurityGroup: ec2.SecurityGroup;
  public readonly cloudflarePrefixListV4: ec2.CfnPrefixList;
  public readonly cloudflarePrefixListV6: ec2.CfnPrefixList;

  constructor(scope: Construct, id: string, props: NetworkStackProps) {
    super(scope, id, props);

    const { config } = props;

    this.vpc = new ec2.Vpc(this, 'Vpc', {
      maxAzs: 2,
      natGateways: 0,
      ipAddresses: ec2.IpAddresses.cidr('10.40.0.0/16'),
      subnetConfiguration: [
        { name: 'public', subnetType: ec2.SubnetType.PUBLIC, cidrMask: 24 },
        { name: 'private', subnetType: ec2.SubnetType.PRIVATE_ISOLATED, cidrMask: 22 },
      ],
      restrictDefaultSecurityGroup: true,
    });

    this.cloudflarePrefixListV4 = new ec2.CfnPrefixList(this, 'CloudflareIpv4', {
      prefixListName: `${config.appName}-cloudflare-ipv4`,
      addressFamily: 'IPv4',
      maxEntries: config.cloudflare.maxEntries,
      entries: config.cloudflare.initialIpv4.map((cidr) => ({ cidr, description: 'cloudflare' })),
    });

    this.cloudflarePrefixListV6 = new ec2.CfnPrefixList(this, 'CloudflareIpv6', {
      prefixListName: `${config.appName}-cloudflare-ipv6`,
      addressFamily: 'IPv6',
      maxEntries: config.cloudflare.maxEntries,
      entries: config.cloudflare.initialIpv6.map((cidr) => ({ cidr, description: 'cloudflare' })),
    });

    this.albSecurityGroup = new ec2.SecurityGroup(this, 'AlbSg', {
      vpc: this.vpc,
      description: 'ALB - accepts HTTPS from Cloudflare IP ranges only',
      allowAllOutbound: false,
    });
    this.albSecurityGroup.addIngressRule(
      ec2.Peer.prefixList(this.cloudflarePrefixListV4.attrPrefixListId),
      ec2.Port.tcp(443),
      'Cloudflare IPv4',
    );
    this.albSecurityGroup.addIngressRule(
      ec2.Peer.prefixList(this.cloudflarePrefixListV6.attrPrefixListId),
      ec2.Port.tcp(443),
      'Cloudflare IPv6',
    );

    this.ecsSecurityGroup = new ec2.SecurityGroup(this, 'EcsSg', {
      vpc: this.vpc,
      description: 'ECS Fargate tasks - accepts traffic from ALB SG only',
      allowAllOutbound: true,
    });
    this.ecsSecurityGroup.addIngressRule(
      this.albSecurityGroup,
      ec2.Port.tcp(config.ecs.containerPort),
      'ALB to container port',
    );
    this.albSecurityGroup.addEgressRule(
      this.ecsSecurityGroup,
      ec2.Port.tcp(config.ecs.containerPort),
      'ALB to ECS container port',
    );

    this.endpointSecurityGroup = new ec2.SecurityGroup(this, 'VpcEndpointSg', {
      vpc: this.vpc,
      description: 'Interface VPC endpoints - accepts HTTPS from ECS SG',
      allowAllOutbound: false,
    });
    this.endpointSecurityGroup.addIngressRule(
      this.ecsSecurityGroup,
      ec2.Port.tcp(443),
      'ECS tasks to interface endpoints',
    );

    new ec2.GatewayVpcEndpoint(this, 'S3GatewayEndpoint', {
      vpc: this.vpc,
      service: ec2.GatewayVpcEndpointAwsService.S3,
      subnets: [{ subnetType: ec2.SubnetType.PRIVATE_ISOLATED }],
    });

    new ec2.GatewayVpcEndpoint(this, 'DynamoDbGatewayEndpoint', {
      vpc: this.vpc,
      service: ec2.GatewayVpcEndpointAwsService.DYNAMODB,
      subnets: [{ subnetType: ec2.SubnetType.PRIVATE_ISOLATED }],
    });

    const privateSubnetAzA = this.vpc.selectSubnets({
      subnetType: ec2.SubnetType.PRIVATE_ISOLATED,
      availabilityZones: [this.vpc.availabilityZones[0]],
    });

    const interfaceEndpoints: Array<[string, ec2.InterfaceVpcEndpointAwsService]> = [
      ['BedrockRuntimeEndpoint', ec2.InterfaceVpcEndpointAwsService.BEDROCK_RUNTIME],
      ['EcrApiEndpoint', ec2.InterfaceVpcEndpointAwsService.ECR],
      ['EcrDkrEndpoint', ec2.InterfaceVpcEndpointAwsService.ECR_DOCKER],
      ['CloudWatchLogsEndpoint', ec2.InterfaceVpcEndpointAwsService.CLOUDWATCH_LOGS],
    ];

    for (const [logicalId, service] of interfaceEndpoints) {
      new ec2.InterfaceVpcEndpoint(this, logicalId, {
        vpc: this.vpc,
        service,
        subnets: privateSubnetAzA,
        securityGroups: [this.endpointSecurityGroup],
        privateDnsEnabled: true,
      });
    }

    this.buildCloudflareSync(config);
  }

  private buildCloudflareSync(config: AppConfig): void {
    const fnLogGroup = new logs.LogGroup(this, 'CloudflareIpSyncLogs', {
      retention: logs.RetentionDays.ONE_MONTH,
      removalPolicy: RemovalPolicy.DESTROY,
    });

    const fn = new lambda.Function(this, 'CloudflareIpSyncFn', {
      runtime: lambda.Runtime.NODEJS_22_X,
      handler: 'index.handler',
      code: lambda.Code.fromAsset(path.join(__dirname, '..', 'lambda', 'cloudflare-sync')),
      timeout: Duration.seconds(30),
      memorySize: 256,
      logGroup: fnLogGroup,
      environment: {
        PREFIX_LIST_ID_V4: this.cloudflarePrefixListV4.attrPrefixListId,
        PREFIX_LIST_ID_V6: this.cloudflarePrefixListV6.attrPrefixListId,
      },
    });

    const prefixListArns = [
      this.formatArn({
        service: 'ec2',
        resource: 'prefix-list',
        resourceName: this.cloudflarePrefixListV4.attrPrefixListId,
      }),
      this.formatArn({
        service: 'ec2',
        resource: 'prefix-list',
        resourceName: this.cloudflarePrefixListV6.attrPrefixListId,
      }),
    ];

    fn.addToRolePolicy(
      new iam.PolicyStatement({
        actions: ['ec2:DescribeManagedPrefixLists'],
        resources: ['*'],
      }),
    );
    fn.addToRolePolicy(
      new iam.PolicyStatement({
        actions: ['ec2:GetManagedPrefixListEntries', 'ec2:ModifyManagedPrefixList'],
        resources: prefixListArns,
      }),
    );

    new events.Rule(this, 'CloudflareIpSyncSchedule', {
      schedule: events.Schedule.expression(config.cloudflare.syncSchedule),
      targets: [new eventTargets.LambdaFunction(fn, { retryAttempts: 2 })],
      description: 'Weekly Cloudflare IP range sync',
    });

    fn.applyRemovalPolicy(RemovalPolicy.DESTROY);
  }
}
