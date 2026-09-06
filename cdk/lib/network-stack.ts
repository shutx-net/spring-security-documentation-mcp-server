import {
  Stack,
  StackProps,
  aws_ec2 as ec2,
} from 'aws-cdk-lib';
import { Construct } from 'constructs';

export class NetworkStack extends Stack {
  public readonly vpc: ec2.IVpc;

  constructor(scope: Construct, id: string, props: StackProps) {
    super(scope, id, props);

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
  }
}
