import type { S3Profile } from '../api/types';
import type { S3FormValues } from './s3Form';

/**
 * Presets fill the form; only the resulting fields are saved, so a profile's provider is
 * recognised again from its endpoint.
 */
export type S3Provider =
  /** AWS itself: no endpoint, the bucket's region is discovered. */
  | { kind: 'aws'; label: string }
  /** The endpoint is derived from one of a fixed set of regions. */
  | {
      kind: 'regional';
      label: string;
      regions: readonly [string, ...string[]];
      endpoint: (region: string) => string;
      pathStyle: boolean;
    }
  /** The endpoint is per account or per host, so it stays editable. */
  | {
      kind: 'custom';
      label: string;
      region: string;
      endpoint: string;
      placeholder: string;
      pathStyle: boolean;
      matches: (host: string) => boolean;
    }
  /** Every field by hand. */
  | { kind: 'manual'; label: string };

export type S3ProviderId =
  | 'aws'
  | 'b2'
  | 'r2'
  | 'digitalocean'
  | 'hetzner'
  | 'idrive'
  | 'scaleway'
  | 'wasabi'
  | 'local'
  | 'other';

export const S3_PROVIDERS: Readonly<Record<S3ProviderId, S3Provider>> = {
  aws: { kind: 'aws', label: 'Amazon S3' },
  b2: {
    kind: 'regional',
    label: 'Backblaze B2',
    regions: [
      'us-west-004',
      'us-west-000',
      'us-west-001',
      'us-west-002',
      'us-east-005',
      'eu-central-003',
    ],
    endpoint: (r) => `https://s3.${r}.backblazeb2.com`,
    pathStyle: false,
  },
  r2: {
    kind: 'custom',
    label: 'Cloudflare R2',
    region: 'auto',
    endpoint: '',
    placeholder: 'https://<account-id>.r2.cloudflarestorage.com',
    pathStyle: true,
    matches: (host) => host.endsWith('.r2.cloudflarestorage.com'),
  },
  digitalocean: {
    kind: 'regional',
    label: 'DigitalOcean Spaces',
    regions: ['nyc3', 'sfo3', 'atl1', 'tor1', 'ams3', 'fra1', 'lon1', 'sgp1', 'blr1', 'syd1'],
    endpoint: (r) => `https://${r}.digitaloceanspaces.com`,
    pathStyle: false,
  },
  hetzner: {
    kind: 'regional',
    label: 'Hetzner Object Storage',
    regions: ['fsn1', 'nbg1', 'hel1'],
    endpoint: (r) => `https://${r}.your-objectstorage.com`,
    pathStyle: false,
  },
  idrive: {
    kind: 'regional',
    label: 'IDrive e2',
    regions: [
      'us-east-1',
      'us-southeast-1',
      'us-central-1',
      'us-midwest-1',
      'us-southwest-1',
      'us-west-1',
      'us-west-2',
      'us-west-3',
      'us-west-4',
      'ca-east-1',
      'eu-west-1',
      'eu-west-2',
      'eu-west-3',
      'eu-west-4',
      'eu-central-1',
      'eu-central-2',
      'eu-south-1',
      'ap-southeast-1',
      'ap-southeast-2',
      'ap-northeast-1',
    ],
    endpoint: (r) => `https://s3.${r}.idrivee2.com`,
    pathStyle: true,
  },
  scaleway: {
    kind: 'regional',
    label: 'Scaleway',
    regions: ['fr-par', 'nl-ams', 'pl-waw'],
    endpoint: (r) => `https://s3.${r}.scw.cloud`,
    pathStyle: false,
  },
  wasabi: {
    kind: 'regional',
    label: 'Wasabi',
    regions: [
      'us-east-1',
      'us-east-2',
      'us-central-1',
      'us-west-1',
      'us-west-2',
      'ca-central-1',
      'eu-central-1',
      'eu-central-2',
      'eu-west-1',
      'eu-west-2',
      'eu-west-3',
      'eu-south-1',
      'ap-northeast-1',
      'ap-northeast-2',
      'ap-southeast-1',
      'ap-southeast-2',
    ],
    endpoint: (r) => `https://s3.${r}.wasabisys.com`,
    pathStyle: false,
  },
  local: {
    kind: 'custom',
    label: 'MinIO / RustFS (local)',
    region: 'us-east-1',
    endpoint: 'http://localhost:9000',
    placeholder: 'http://localhost:9000',
    pathStyle: true,
    matches: (host) => host === 'localhost' || host === '127.0.0.1' || host === '[::1]',
  },
  other: { kind: 'manual', label: 'Other S3-compatible' },
};

export const S3_PROVIDER_IDS = Object.keys(S3_PROVIDERS) as S3ProviderId[];

/** Switches the form to a provider's preset, keeping the name unless it was the old default. */
export function applyProvider(f: S3FormValues, id: S3ProviderId): S3FormValues {
  const prev = S3_PROVIDERS[f.provider];
  const next = S3_PROVIDERS[id];
  const untouchedName = f.name.trim() === '' || f.name === prev.label;
  const name = untouchedName ? (next.kind === 'manual' ? '' : next.label) : f.name;
  const base = { ...f, provider: id, name };

  switch (next.kind) {
    case 'manual':
      return base;
    case 'aws':
      return { ...base, region: prev.kind === 'manual' ? f.region : '', endpoint: '' };
    case 'regional': {
      const region = next.regions.includes(f.region) ? f.region : next.regions[0];
      return {
        ...base,
        auth: 'keys',
        region,
        endpoint: next.endpoint(region),
        pathStyle: next.pathStyle,
      };
    }
    case 'custom':
      return {
        ...base,
        auth: 'keys',
        region: next.region,
        endpoint: next.endpoint,
        pathStyle: next.pathStyle,
      };
    default:
      return next satisfies never;
  }
}

/** Sets the region, re-deriving the endpoint for providers that have one per region. */
export function applyRegion(f: S3FormValues, region: string): S3FormValues {
  const p = S3_PROVIDERS[f.provider];
  return p.kind === 'regional' ? { ...f, region, endpoint: p.endpoint(region) } : { ...f, region };
}

/** Recognises a saved profile's provider; anything that isn't an exact preset is manual. */
export function providerOf(p: Pick<S3Profile, 'endpoint' | 'region'>): S3ProviderId {
  const endpoint = p.endpoint ?? '';
  const region = p.region ?? '';
  if (endpoint === '') return 'aws';

  let host: string;
  try {
    host = new URL(endpoint).hostname;
  } catch {
    return 'other';
  }
  const found = S3_PROVIDER_IDS.find((id) => {
    const v = S3_PROVIDERS[id];
    switch (v.kind) {
      case 'regional':
        return v.regions.includes(region) && v.endpoint(region) === endpoint;
      case 'custom':
        return v.matches(host);
      case 'aws':
      case 'manual':
        return false;
      default:
        return v satisfies never;
    }
  });
  return found ?? 'other';
}

/** Where a profile connects, for the profile list. */
export function describeLocation(p: Pick<S3Profile, 'endpoint' | 'region'>): string {
  const v = S3_PROVIDERS[providerOf(p)];
  switch (v.kind) {
    case 'aws':
      return p.region ? `${v.label} ${p.region}` : v.label;
    case 'regional':
      return `${v.label} ${p.region}`;
    case 'custom':
      return v.endpoint === '' ? v.label : (p.endpoint ?? '');
    case 'manual':
      return p.endpoint ?? '';
    default:
      return v satisfies never;
  }
}
