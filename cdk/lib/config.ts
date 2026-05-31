import { App } from 'aws-cdk-lib';

export interface AppConfig {
  appName: string;
  embeddingModelId: string;
  embeddingDimension: number;

  ecs: {
    cpu: number;
    memoryMiB: number;
    containerPort: number;
    desiredCount: number;
    minCapacity: number;
    maxCapacity: number;
    requestsPerTarget: number;
  };

  schedule: string;
  scheduleTimezone: string;

  cloudflare: {
    initialIpv4: string[];
    initialIpv6: string[];
    syncSchedule: string;
    maxEntries: number;
  };

  domain?: {
    domainName: string;
    certificateArn: string;
  };

  githubTokenSecretName?: string;

  github?: {
    org: string;
    repo: string;
  };
}

export function loadConfig(app: App): AppConfig {
  const ctx = <T>(key: string, fallback?: T): T => (app.node.tryGetContext(key) ?? fallback) as T;

  const domainName = ctx<string | null>('domainName', null);
  const certificateArn = ctx<string | null>('certificateArn', null);

  const allDomainSet = domainName && certificateArn;
  const someDomainSet = domainName || certificateArn;
  if (someDomainSet && !allDomainSet) {
    throw new Error(
      'Custom domain requires both: domainName and certificateArn (ACM cert in the same region as ALB).',
    );
  }

  return {
    appName: ctx<string>('appName', 'spring-sec-mcp'),
    embeddingModelId: ctx<string>('embeddingModelId', 'amazon.titan-embed-text-v2:0'),
    embeddingDimension: ctx<number>('embeddingDimension', 1024),
    ecs: ctx<AppConfig['ecs']>('ecs'),
    schedule: ctx<string>('schedule'),
    scheduleTimezone: ctx<string>('scheduleTimezone', 'UTC'),
    cloudflare: ctx<AppConfig['cloudflare']>('cloudflare'),
    domain: allDomainSet
      ? { domainName: domainName!, certificateArn: certificateArn! }
      : undefined,
    githubTokenSecretName: ctx<string | undefined>('githubTokenSecretName', undefined) ?? undefined,
    github: ctx<AppConfig['github']>('github', undefined),
  };
}
