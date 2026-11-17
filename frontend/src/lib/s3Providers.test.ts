import { describe, expect, it } from 'vitest';
import { EMPTY_S3_FORM, formFromProfile, profileFromForm, validateS3Form } from './s3Form';
import {
  S3_PROVIDERS,
  S3_PROVIDER_IDS,
  applyProvider,
  applyRegion,
  describeLocation,
  providerOf,
} from './s3Providers';

describe('applyProvider', () => {
  it('fills a regional preset from its first region and switches to access keys', () => {
    const f = applyProvider(EMPTY_S3_FORM, 'b2');
    expect(f).toMatchObject({
      provider: 'b2',
      name: 'Backblaze B2',
      auth: 'keys',
      region: 'us-west-004',
      endpoint: 'https://s3.us-west-004.backblazeb2.com',
      pathStyle: false,
    });
  });

  it('keeps a region the new provider also offers', () => {
    const wasabi = applyRegion(applyProvider(EMPTY_S3_FORM, 'wasabi'), 'eu-central-1');
    expect(applyProvider(wasabi, 'idrive')).toMatchObject({
      region: 'eu-central-1',
      endpoint: 'https://s3.eu-central-1.idrivee2.com',
      pathStyle: true,
    });
  });

  it('replaces a default name but keeps one the user typed', () => {
    const b2 = applyProvider(EMPTY_S3_FORM, 'b2');
    expect(applyProvider(b2, 'wasabi').name).toBe('Wasabi');
    expect(applyProvider({ ...b2, name: 'Backups' }, 'wasabi').name).toBe('Backups');
    expect(applyProvider(b2, 'other').name).toBe('');
  });

  it('leaves the fields alone when switching to manual entry', () => {
    const local = applyProvider(EMPTY_S3_FORM, 'local');
    const manual = applyProvider(local, 'other');
    expect(manual).toEqual({ ...local, provider: 'other', name: '' });
  });

  it('clears the endpoint for AWS but keeps the credential method', () => {
    const f = applyProvider(
      { ...applyProvider(EMPTY_S3_FORM, 'hetzner'), auth: 'aws-profile' },
      'aws'
    );
    expect(f).toMatchObject({ endpoint: '', region: '', auth: 'aws-profile' });
  });
});

describe('applyRegion', () => {
  it('re-derives a regional endpoint and leaves a custom one alone', () => {
    const spaces = applyRegion(applyProvider(EMPTY_S3_FORM, 'digitalocean'), 'fra1');
    expect(spaces.endpoint).toBe('https://fra1.digitaloceanspaces.com');
    const local = applyRegion(applyProvider(EMPTY_S3_FORM, 'local'), 'eu-west-1');
    expect(local.endpoint).toBe('http://localhost:9000');
  });
});

describe('providerOf', () => {
  it('recognises every preset region after a save', () => {
    for (const id of S3_PROVIDER_IDS) {
      const p = S3_PROVIDERS[id];
      if (p.kind !== 'regional') continue;
      for (const region of p.regions) {
        const form = applyRegion(applyProvider(EMPTY_S3_FORM, id), region);
        expect(formFromProfile(profileFromForm(form)).provider, `${id} ${region}`).toBe(id);
      }
    }
  });

  it('recognises custom endpoints by host', () => {
    expect(
      providerOf({ endpoint: 'https://abc123.r2.cloudflarestorage.com', region: 'auto' })
    ).toBe('r2');
    expect(providerOf({ endpoint: 'http://127.0.0.1:9100' })).toBe('local');
    expect(providerOf({})).toBe('aws');
  });

  it('treats an endpoint that no preset would produce as manual', () => {
    expect(providerOf({ endpoint: 'https://s3.wasabisys.com', region: 'us-east-1' })).toBe('other');
    expect(
      providerOf({ endpoint: 'https://s3.us-west-004.backblazeb2.com', region: 'us-west-002' })
    ).toBe('other');
    expect(providerOf({ endpoint: 'https://minio.lan:9000' })).toBe('other');
  });
});

describe('validateS3Form', () => {
  it('requires the account endpoint for Cloudflare R2', () => {
    const r2 = { ...applyProvider(EMPTY_S3_FORM, 'r2'), accessKeyId: 'k', secret: 's' };
    expect(validateS3Form(r2)).toMatch(/Cloudflare R2 endpoint/);
    expect(validateS3Form({ ...r2, endpoint: 'https://abc.r2.cloudflarestorage.com' })).toBeNull();
  });
});

describe('describeLocation', () => {
  it('names known providers and falls back to the endpoint', () => {
    expect(
      describeLocation({
        endpoint: 'https://s3.eu-central-003.backblazeb2.com',
        region: 'eu-central-003',
      })
    ).toBe('Backblaze B2 eu-central-003');
    expect(describeLocation({ region: 'eu-west-1' })).toBe('Amazon S3 eu-west-1');
    expect(describeLocation({ endpoint: 'http://localhost:9000' })).toBe('http://localhost:9000');
    expect(describeLocation({ endpoint: 'https://minio.lan' })).toBe('https://minio.lan');
  });
});
