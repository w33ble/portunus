import type { S3Profile } from '../api/types';
import { S3_PROVIDERS, type S3ProviderId, providerOf } from './s3Providers';

export type S3Auth = 'aws-profile' | 'keys';

export interface S3FormValues {
  id: string;
  /** Not saved; recognised again from the endpoint when editing. */
  provider: S3ProviderId;
  name: string;
  auth: S3Auth;
  awsProfile: string;
  accessKeyId: string;
  secret: string;
  region: string;
  endpoint: string;
  pathStyle: boolean;
  bucket: string;
}

export const EMPTY_S3_FORM: S3FormValues = {
  id: '',
  provider: 'aws',
  name: '',
  auth: 'aws-profile',
  awsProfile: '',
  accessKeyId: '',
  secret: '',
  region: '',
  endpoint: '',
  pathStyle: false,
  bucket: '',
};

export function formFromProfile(p: S3Profile): S3FormValues {
  return {
    id: p.id,
    provider: providerOf(p),
    name: p.name,
    auth: p.profile ? 'aws-profile' : p.accessKeyId ? 'keys' : 'aws-profile',
    awsProfile: p.profile ?? '',
    accessKeyId: p.accessKeyId ?? '',
    secret: '',
    region: p.region ?? '',
    endpoint: p.endpoint ?? '',
    pathStyle: p.pathStyle ?? false,
    bucket: p.bucket ?? '',
  };
}

const t = (s: string) => s.trim();

/** Only the credentials of the chosen method are sent; the other method's fields are dropped. */
export function profileFromForm(f: S3FormValues): S3Profile {
  return {
    id: f.id,
    name: t(f.name),
    ...(f.auth === 'aws-profile'
      ? { profile: t(f.awsProfile) }
      : { accessKeyId: t(f.accessKeyId) }),
    ...(t(f.region) && { region: t(f.region) }),
    ...(t(f.endpoint) && { endpoint: t(f.endpoint) }),
    ...(f.pathStyle && { pathStyle: true }),
    ...(t(f.bucket) && { bucket: t(f.bucket) }),
  };
}

/** Returns an error message, or null when the form can be saved. */
export function validateS3Form(f: S3FormValues): string | null {
  if (f.name.trim() === '') return 'Give the profile a name.';
  if (f.auth === 'keys') {
    if (f.accessKeyId.trim() === '') return 'Enter the access key ID.';
    if (f.id === '' && f.secret === '') return 'Enter the secret access key.';
  }
  const endpoint = f.endpoint.trim();
  const provider = S3_PROVIDERS[f.provider];
  if (provider.kind === 'custom' && endpoint === '') {
    return `Enter the ${provider.label} endpoint.`;
  }
  if (endpoint !== '' && !/^https?:\/\/\S+$/i.test(endpoint)) {
    return 'The endpoint must start with http:// or https://.';
  }
  return null;
}

/** An AWS profile name may be left blank to use the default credential chain. */
export function secretFor(f: S3FormValues): string {
  return f.auth === 'keys' ? f.secret : '';
}
