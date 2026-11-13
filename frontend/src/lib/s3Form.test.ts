import { describe, expect, it } from 'vitest';
import {
  EMPTY_S3_FORM,
  type S3FormValues,
  formFromProfile,
  profileFromForm,
  secretFor,
  validateS3Form,
} from './s3Form';

const form = (over: Partial<S3FormValues>): S3FormValues => ({ ...EMPTY_S3_FORM, ...over });

describe('profileFromForm', () => {
  it('sends only the chosen credential method and non-empty fields', () => {
    const p = profileFromForm(
      form({
        name: ' Backups ',
        auth: 'keys',
        accessKeyId: 'AKIA',
        awsProfile: 'stale',
        region: ' us-west-2 ',
      })
    );
    expect(p).toEqual({ id: '', name: 'Backups', accessKeyId: 'AKIA', region: 'us-west-2' });
  });

  it('keeps path-style and bucket for AWS profiles', () => {
    const p = profileFromForm(form({ name: 'n', awsProfile: 'dev', pathStyle: true, bucket: 'b' }));
    expect(p).toEqual({ id: '', name: 'n', profile: 'dev', pathStyle: true, bucket: 'b' });
  });

  it('round-trips through formFromProfile', () => {
    const profile = {
      id: 'x',
      name: 'n',
      accessKeyId: 'AKIA',
      endpoint: 'https://minio.local',
      pathStyle: true,
    };
    expect(profileFromForm(formFromProfile(profile))).toEqual(profile);
  });
});

describe('validateS3Form', () => {
  it('requires a name', () => {
    expect(validateS3Form(form({}))).toMatch(/name/i);
  });
  it('requires a secret for new key-based profiles but not when editing', () => {
    const base = form({ name: 'n', auth: 'keys', accessKeyId: 'AKIA' });
    expect(validateS3Form(base)).toMatch(/secret/i);
    expect(validateS3Form({ ...base, id: 'existing' })).toBeNull();
    expect(validateS3Form({ ...base, secret: 's' })).toBeNull();
  });
  it('checks the endpoint scheme', () => {
    expect(validateS3Form(form({ name: 'n', endpoint: 'minio:9000' }))).toMatch(/http/);
    expect(validateS3Form(form({ name: 'n', endpoint: 'http://localhost:9000' }))).toBeNull();
  });
});

describe('secretFor', () => {
  it('never sends a secret for AWS profile auth', () => {
    expect(secretFor(form({ auth: 'aws-profile', secret: 'leftover' }))).toBe('');
    expect(secretFor(form({ auth: 'keys', secret: 's' }))).toBe('s');
  });
});
