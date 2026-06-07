#!/usr/bin/env node
import 'source-map-support/register';
import * as cdk from 'aws-cdk-lib';
import { loadConfig } from '../lib/config';
import { StorageStack } from '../lib/storage-stack';
import { NetworkStack } from '../lib/network-stack';
import { ServiceStack } from '../lib/service-stack';
import { PipelineStack } from '../lib/pipeline-stack';
import { CicdStack } from '../lib/cicd-stack';

const app = new cdk.App();
const config = loadConfig(app);

const env: cdk.Environment = {
  account: process.env.CDK_DEFAULT_ACCOUNT,
  region: process.env.CDK_DEFAULT_REGION,
};

const prefix = config.appName;

const storage = new StorageStack(app, `${prefix}-storage`, { env, config });
const network = new NetworkStack(app, `${prefix}-network`, { env, config });
if (config.domain) {
  const service = new ServiceStack(app, `${prefix}-service`, {
    env,
    config,
    vpc: network.vpc,
    albSecurityGroup: network.albSecurityGroup,
    ecsSecurityGroup: network.ecsSecurityGroup,
    contentBucket: storage.contentBucket,
    vectorBucket: storage.vectorBucket,
    vectorIndex: storage.vectorIndex,
    tables: storage.tables,
  });
  service.addDependency(network);
  service.addDependency(storage);

  if (config.github) {
    const cicd = new CicdStack(app, `${prefix}-cicd`, {
      env,
      githubOrg: config.github.org,
      githubRepo: config.github.repo,
      ecrRepository: service.ecrRepository,
      ecsServiceArn: service.ecsServiceArn,
      indexerRepository: storage.indexerRepository,
      config,
      vectorBucket: storage.vectorBucket,
      vectorIndex: storage.vectorIndex,
      tables: storage.tables,
    });
    cicd.addDependency(service);
  }
}

const pipeline = new PipelineStack(app, `${prefix}-pipeline`, {
  env,
  config,
  vpc: network.vpc,
  contentBucket: storage.contentBucket,
  vectorBucket: storage.vectorBucket,
  vectorIndex: storage.vectorIndex,
  tables: storage.tables,
  chunksTableGsiName: storage.chunksTableGsiName,
  indexerRepository: storage.indexerRepository,
});
pipeline.addDependency(storage);
pipeline.addDependency(network);

cdk.Tags.of(app).add('app', config.appName);
cdk.Tags.of(app).add('managed-by', 'cdk');
