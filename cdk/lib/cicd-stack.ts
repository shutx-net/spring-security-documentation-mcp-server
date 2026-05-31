import {
  Stack,
  StackProps,
  CfnOutput,
  aws_iam as iam,
  aws_ecr as ecr,
} from 'aws-cdk-lib';
import { Construct } from 'constructs';

export interface CicdStackProps extends StackProps {
  readonly githubOrg: string;
  readonly githubRepo: string;
  readonly ecrRepository: ecr.IRepository;
}

export class CicdStack extends Stack {
  constructor(scope: Construct, id: string, props: CicdStackProps) {
    super(scope, id, props);

    const { githubOrg, githubRepo, ecrRepository } = props;

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

    // grantPush adds the 7 ECR repository-level actions needed for docker push.
    ecrRepository.grantPush(role);

    // GetAuthorizationToken operates on * (not a specific repository).
    role.addToPolicy(new iam.PolicyStatement({
      actions: ['ecr:GetAuthorizationToken'],
      resources: ['*'],
    }));

    new CfnOutput(this, 'GitHubActionsRoleArn', {
      value: role.roleArn,
      description: 'Set as AWS_ROLE_ARN secret in GitHub repository settings',
    });
  }
}
