import { useState } from 'react';
import { toAppError } from '../../api/errors';
import type { S3Profile } from '../../api/types';
import {
  EMPTY_S3_FORM,
  type S3FormValues,
  formFromProfile,
  profileFromForm,
  secretFor,
  validateS3Form,
} from '../../lib/s3Form';
import { useConnections } from '../../stores/connections';
import controls from '../ui/controls.module.css';
import { Icon } from '../ui/Icon';
import { Modal } from '../ui/Modal';
import { S3ProfileForm } from './S3ProfileForm';
import styles from './S3ProfilesDialog.module.css';

function describeProfile(p: S3Profile): string {
  const auth = p.profile
    ? `AWS profile ${p.profile}`
    : p.accessKeyId
      ? 'Access key'
      : 'Default credentials';
  const where = p.endpoint || p.region || 'AWS';
  return `${auth} · ${where}${p.bucket ? ` · ${p.bucket}` : ''}`;
}

export function S3ProfilesDialog({ onClose }: { onClose: () => void }) {
  const profiles = useConnections((s) => s.profiles);
  const sessionOnly = useConnections((s) => s.secretsSessionOnly);
  const [form, setForm] = useState<S3FormValues | null>(null);
  const [confirmId, setConfirmId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const run = async (work: () => Promise<void>) => {
    setBusy(true);
    setError(null);
    try {
      await work();
    } catch (e) {
      setError(toAppError(e).message);
    } finally {
      setBusy(false);
    }
  };

  const save = () => {
    if (form === null) return;
    const invalid = validateS3Form(form);
    if (invalid) {
      setError(invalid);
      return;
    }
    void run(async () => {
      await useConnections.getState().saveProfile(profileFromForm(form), secretFor(form));
      setForm(null);
    });
  };

  const remove = (id: string) =>
    run(async () => {
      await useConnections.getState().deleteProfile(id);
      setConfirmId(null);
    });

  const notice = sessionOnly && (
    <p className={controls.notice}>
      No system keychain is available, so saved secret keys are kept for this session only.
    </p>
  );

  if (form !== null) {
    return (
      <Modal
        title={form.id === '' ? 'New S3 profile' : 'Edit S3 profile'}
        width={460}
        onCancel={() => {
          setForm(null);
          setError(null);
        }}
        onSubmit={save}
        actions={
          <>
            <button
              type="button"
              className={controls.button}
              onClick={() => {
                setForm(null);
                setError(null);
              }}
            >
              Back
            </button>
            <button
              type="submit"
              className={`${controls.button} ${controls.primary}`}
              disabled={busy}
            >
              Save
            </button>
          </>
        }
      >
        {notice}
        <S3ProfileForm
          values={form}
          onChange={(next) => {
            setForm(next);
            setError(null);
          }}
        />
        {error && <p className={controls.errorText}>{error}</p>}
      </Modal>
    );
  }

  return (
    <Modal
      title="S3 profiles"
      width={460}
      onCancel={onClose}
      onSubmit={onClose}
      actions={
        <>
          <button
            type="button"
            className={controls.button}
            onClick={() => setForm({ ...EMPTY_S3_FORM })}
          >
            Add profile…
          </button>
          <button type="submit" className={`${controls.button} ${controls.primary}`}>
            Done
          </button>
        </>
      }
    >
      {notice}
      {profiles.length === 0 ? (
        <p className={controls.hint}>
          No profiles yet. Add one for AWS or any S3-compatible service.
        </p>
      ) : (
        <ul className={styles.list}>
          {profiles.map((p) => (
            <li key={p.id} className={styles.item}>
              <Icon name="cloud" />
              <div className={styles.text}>
                <span className={styles.name}>{p.name}</span>
                <span className={styles.meta}>{describeProfile(p)}</span>
              </div>
              {confirmId === p.id ? (
                <>
                  <button
                    type="button"
                    className={`${controls.button} ${controls.danger}`}
                    disabled={busy}
                    onClick={() => void remove(p.id)}
                  >
                    Delete
                  </button>
                  <button
                    type="button"
                    className={controls.button}
                    onClick={() => setConfirmId(null)}
                  >
                    Keep
                  </button>
                </>
              ) : (
                <>
                  <button
                    type="button"
                    className={controls.iconButton}
                    aria-label={`Edit ${p.name}`}
                    title="Edit"
                    onClick={() => setForm(formFromProfile(p))}
                  >
                    <Icon name="edit" />
                  </button>
                  <button
                    type="button"
                    className={controls.iconButton}
                    aria-label={`Delete ${p.name}`}
                    title="Delete"
                    onClick={() => setConfirmId(p.id)}
                  >
                    <Icon name="trash" />
                  </button>
                </>
              )}
            </li>
          ))}
        </ul>
      )}
      {error && <p className={controls.errorText}>{error}</p>}
    </Modal>
  );
}
