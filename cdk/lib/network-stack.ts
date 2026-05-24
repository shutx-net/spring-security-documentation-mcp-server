import {
  Stack,
  StackProps,
  aws_ec2 as ec2,
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

    this.albSecurityGroup = new ec2.SecurityGroup(this, 'AlbSg', {
      vpc: this.vpc,
      description: 'ALB — accepts traffic from CloudFront origin-facing prefix list only',
      allowAllOutbound: false,
    });
    this.albSecurityGroup.addIngressRule(
      ec2.Peer.prefixList(config.cloudfrontOriginPrefixListId),
      ec2.Port.tcp(80),
      'CloudFront origin-facing prefix list (HTTP)',
    );

    this.ecsSecurityGroup = new ec2.SecurityGroup(this, 'EcsSg', {
      vpc: this.vpc,
      description: 'ECS Fargate tasks — accepts traffic from ALB SG only',
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
      description: 'Interface VPC endpoints — accepts HTTPS from ECS SG',
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
  }
}
