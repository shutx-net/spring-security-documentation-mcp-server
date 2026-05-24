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

  waf: {
    rateLimitPer5Min: number;
    maxBodyBytes: number;
  };

  schedule: string;
  scheduleTimezone: string;

  cloudfrontOriginPrefixListId: string;

  domain?: {
    domainName: string;
    hostedZoneId: string;
    hostedZoneName: string;
    certificateArn: string;
  };
}

export function loadConfig(app: App): AppConfig {
  const ctx = <T>(key: string, fallback?: T): T => (app.node.tryGetContext(key) ?? fallback) as T;

  const domainName = ctx<string | null>('domainName', null);
  const hostedZoneId = ctx<string | null>('hostedZoneId', null);
  const hostedZoneName = ctx<string | null>('hostedZoneName', null);
  const certificateArn = ctx<string | null>('certificateArn', null);

  const allDomainSet = domainName && hostedZoneId && hostedZoneName && certificateArn;
  const someDomainSet = domainName || hostedZoneId || hostedZoneName || certificateArn;
  if (someDomainSet && !allDomainSet) {
    throw new Error(
      'Custom domain requires all of: domainName, hostedZoneId, hostedZoneName, certificateArn (certificate must be in us-east-1).',
    );
  }

  return {
    appName: ctx<string>('appName', 'spring-sec-mcp'),
    embeddingModelId: ctx<string>('embeddingModelId', 'amazon.titan-embed-text-v2:0'),
    embeddingDimension: ctx<number>('embeddingDimension', 1024),
    ecs: ctx<AppConfig['ecs']>('ecs'),
    waf: ctx<AppConfig['waf']>('waf'),
    schedule: ctx<string>('schedule'),
    scheduleTimezone: ctx<string>('scheduleTimezone', 'UTC'),
    cloudfrontOriginPrefixListId: ctx<string>('cloudfrontOriginPrefixListId'),
    domain: allDomainSet
      ? {
          domainName: domainName!,
          hostedZoneId: hostedZoneId!,
          hostedZoneName: hostedZoneName!,
          certificateArn: certificateArn!,
        }
      : undefined,
  };
}
