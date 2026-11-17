import type { S3FormValues } from '../../lib/s3Form';
import {
  S3_PROVIDERS,
  S3_PROVIDER_IDS,
  type S3ProviderId,
  applyProvider,
  applyRegion,
} from '../../lib/s3Providers';
import controls from '../ui/controls.module.css';
import { Select } from '../ui/Select';
import styles from './S3ProfilesDialog.module.css';

interface Props {
  values: S3FormValues;
  onChange: (next: S3FormValues) => void;
}

export function S3ProfileForm({ values, onChange }: Props) {
  const set = <K extends keyof S3FormValues>(key: K, value: S3FormValues[K]) =>
    onChange({ ...values, [key]: value });
  const editing = values.id !== '';
  const provider = S3_PROVIDERS[values.provider];

  return (
    <>
      <label className={controls.label}>
        Provider
        <Select
          value={values.provider}
          autoFocus={!editing}
          onChange={(e) => onChange(applyProvider(values, e.target.value as S3ProviderId))}
        >
          {S3_PROVIDER_IDS.map((id) => (
            <option key={id} value={id}>
              {S3_PROVIDERS[id].label}
            </option>
          ))}
        </Select>
      </label>

      <label className={controls.label}>
        Name
        <input
          className={controls.field}
          value={values.name}
          autoFocus={editing}
          onChange={(e) => set('name', e.target.value)}
        />
      </label>

      <fieldset className={styles.fieldset}>
        <legend>Credentials</legend>
        <label className={controls.checkbox}>
          <input
            type="radio"
            name="s3-auth"
            checked={values.auth === 'aws-profile'}
            onChange={() => set('auth', 'aws-profile')}
          />
          AWS shared profile (~/.aws)
        </label>
        <label className={controls.checkbox}>
          <input
            type="radio"
            name="s3-auth"
            checked={values.auth === 'keys'}
            onChange={() => set('auth', 'keys')}
          />
          Access key
        </label>
      </fieldset>

      {values.auth === 'aws-profile' ? (
        <label className={controls.label}>
          AWS profile name
          <input
            className={controls.field}
            value={values.awsProfile}
            placeholder="default"
            spellCheck={false}
            onChange={(e) => set('awsProfile', e.target.value)}
          />
        </label>
      ) : (
        <>
          <label className={controls.label}>
            Access key ID
            <input
              className={controls.field}
              value={values.accessKeyId}
              spellCheck={false}
              autoComplete="off"
              onChange={(e) => set('accessKeyId', e.target.value)}
            />
          </label>
          <label className={controls.label}>
            Secret access key
            <input
              className={controls.field}
              type="password"
              value={values.secret}
              autoComplete="new-password"
              placeholder={editing ? 'Leave blank to keep the saved secret' : ''}
              onChange={(e) => set('secret', e.target.value)}
            />
          </label>
        </>
      )}

      <div className={styles.pair}>
        <label className={controls.label}>
          Region
          {provider.kind === 'regional' ? (
            <Select
              value={values.region}
              onChange={(e) => onChange(applyRegion(values, e.target.value))}
            >
              {provider.regions.map((r) => (
                <option key={r} value={r}>
                  {r}
                </option>
              ))}
            </Select>
          ) : (
            <input
              className={controls.field}
              value={values.region}
              placeholder="us-east-1"
              spellCheck={false}
              onChange={(e) => set('region', e.target.value)}
            />
          )}
        </label>
        <label className={controls.label}>
          Bucket (optional)
          <input
            className={controls.field}
            value={values.bucket}
            placeholder="List all buckets"
            spellCheck={false}
            onChange={(e) => set('bucket', e.target.value)}
          />
        </label>
      </div>

      {provider.kind === 'regional' && (
        <p className={controls.hint}>
          Endpoint <span className={controls.mono}>{values.endpoint}</span>
        </p>
      )}
      {(provider.kind === 'custom' || provider.kind === 'manual') && (
        <label className={controls.label}>
          {provider.kind === 'custom' ? 'Endpoint' : 'Endpoint (optional)'}
          <input
            className={controls.field}
            value={values.endpoint}
            placeholder={
              provider.kind === 'custom'
                ? provider.placeholder
                : 'https://s3.example.com (blank for AWS)'
            }
            spellCheck={false}
            autoCapitalize="off"
            onChange={(e) => set('endpoint', e.target.value)}
          />
        </label>
      )}

      <label className={controls.checkbox}>
        <input
          type="checkbox"
          checked={values.pathStyle}
          onChange={(e) => set('pathStyle', e.target.checked)}
        />
        Use path-style addressing (MinIO and many S3-compatible services)
      </label>
    </>
  );
}
