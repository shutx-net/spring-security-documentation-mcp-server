import {
  EC2Client,
  DescribeManagedPrefixListsCommand,
  GetManagedPrefixListEntriesCommand,
  ModifyManagedPrefixListCommand,
} from '@aws-sdk/client-ec2';

const ec2 = new EC2Client({});

const CLOUDFLARE_URLS = {
  ipv4: 'https://www.cloudflare.com/ips-v4',
  ipv6: 'https://www.cloudflare.com/ips-v6',
};

async function fetchCidrs(url) {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), 10_000);
  try {
    const res = await fetch(url, {
      headers: { 'User-Agent': 'cdk-cloudflare-sync' },
      signal: controller.signal,
    });
    if (!res.ok) throw new Error(`fetch ${url} failed: ${res.status}`);
    return (await res.text())
      .split('\n')
      .map((s) => s.trim())
      .filter((s) => s.length > 0);
  } finally {
    clearTimeout(timeout);
  }
}

async function syncPrefixList(prefixListId, desiredCidrs) {
  const desc = await ec2.send(
    new DescribeManagedPrefixListsCommand({ PrefixListIds: [prefixListId] }),
  );
  const pl = desc.PrefixLists?.[0];
  if (!pl) throw new Error(`prefix list ${prefixListId} not found`);
  const currentVersion = pl.Version;

  const entriesResp = await ec2.send(
    new GetManagedPrefixListEntriesCommand({ PrefixListId: prefixListId }),
  );
  const current = (entriesResp.Entries ?? []).map((e) => e.Cidr);

  const desiredSet = new Set(desiredCidrs);
  const currentSet = new Set(current);
  const toAdd = [...desiredSet].filter((c) => !currentSet.has(c));
  const toRemove = [...currentSet].filter((c) => !desiredSet.has(c));

  if (toAdd.length === 0 && toRemove.length === 0) {
    return { changed: false, current: current.length };
  }

  await ec2.send(
    new ModifyManagedPrefixListCommand({
      PrefixListId: prefixListId,
      CurrentVersion: currentVersion,
      AddEntries: toAdd.length > 0 ? toAdd.map((c) => ({ Cidr: c, Description: 'cloudflare' })) : undefined,
      RemoveEntries: toRemove.length > 0 ? toRemove.map((c) => ({ Cidr: c })) : undefined,
    }),
  );

  return { changed: true, added: toAdd, removed: toRemove };
}

export const handler = async () => {
  const v4Pl = process.env.PREFIX_LIST_ID_V4;
  const v6Pl = process.env.PREFIX_LIST_ID_V6;
  if (!v4Pl || !v6Pl) throw new Error('PREFIX_LIST_ID_V4 / PREFIX_LIST_ID_V6 env required');

  const [ipv4, ipv6] = await Promise.all([
    fetchCidrs(CLOUDFLARE_URLS.ipv4),
    fetchCidrs(CLOUDFLARE_URLS.ipv6),
  ]);

  const [v4Result, v6Result] = await Promise.all([
    syncPrefixList(v4Pl, ipv4),
    syncPrefixList(v6Pl, ipv6),
  ]);

  console.log(JSON.stringify({ v4: v4Result, v6: v6Result }));
  return { v4: v4Result, v6: v6Result };
};
