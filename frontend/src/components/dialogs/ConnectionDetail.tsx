import type { ConnSource } from '../../lib/catalog';
import { useConnections } from '../../stores/connections';
import controls from '../ui/controls.module.css';
import { BookmarksEditor } from './BookmarksEditor';
import { ConfigHostDetails } from './ConfigHostDetails';
import { ConnectionPrefsFields } from './ConnectionPrefsFields';
import styles from './ConnectionsDialog.module.css';
import { S3ProfileForm } from './S3ProfileForm';
import { SavedHostFields } from './SavedHostFields';
import type { ConnectionEditor } from './useConnectionEditor';

function sourceLine(source: ConnSource): string {
  switch (source) {
    case 'local':
      return 'This computer';
    case 'config':
      return 'SSH · from ~/.ssh/config';
    case 'saved':
      return 'SSH · saved in Portunus';
    case 's3':
      return 'S3 profile';
    default: {
      const unreachable: never = source;
      return unreachable;
    }
  }
}

function IdentitySection({ editor }: { editor: ConnectionEditor }) {
  const sessionOnly = useConnections((s) => s.secretsSessionOnly);
  const { draft, conn } = editor;
  const { identity } = draft;

  switch (identity.kind) {
    case 'fixed':
      return conn?.source === 'config' ? <ConfigHostDetails alias={conn.name} /> : null;
    case 'host':
      return (
        <SavedHostFields
          fields={identity.fields}
          onChange={(fields) => editor.setIdentity({ kind: 'host', fields })}
        />
      );
    case 's3':
      return (
        <div className={styles.fields}>
          {sessionOnly && (
            <p className={controls.notice}>
              No system keychain is available, so saved secret keys are kept for this session only.
            </p>
          )}
          <S3ProfileForm
            values={identity.form}
            onChange={(form) => editor.setIdentity({ kind: 's3', form })}
          />
        </div>
      );
    default: {
      const unreachable: never = identity;
      return unreachable;
    }
  }
}

/** The right-hand side: who the connection is, then the app's settings for it. */
export function ConnectionDetail({ editor }: { editor: ConnectionEditor }) {
  const { conn, draft, label, busy } = editor;
  const source: ConnSource = conn?.source ?? (draft.identity.kind === 's3' ? 's3' : 'saved');

  return (
    <section className={styles.detail} aria-label={label}>
      <header className={styles.detailHeader}>
        <div className={styles.titleBlock}>
          <h3 className={styles.detailName}>{label}</h3>
          <p className={styles.source}>{sourceLine(source)}</p>
        </div>
        {source !== 'local' && (
          <label className={controls.checkbox}>
            <input
              type="checkbox"
              checked={!draft.prefs.hideInPicker}
              onChange={(e) => editor.setPrefs({ hideInPicker: !e.target.checked })}
            />
            In picker
          </label>
        )}
        <button
          type="button"
          className={controls.button}
          disabled={conn === null || busy}
          onClick={editor.connect}
        >
          Connect
        </button>
      </header>
      <div className={styles.detailBody}>
        <IdentitySection editor={editor} />
        <h4 className={styles.sectionTitle}>Settings</h4>
        <ConnectionPrefsFields prefs={draft.prefs} onChange={editor.setPrefs} />
        <h4 className={styles.sectionTitle}>Bookmarks</h4>
        <BookmarksEditor
          connId={conn?.id ?? null}
          bookmarks={draft.bookmarks}
          onChange={editor.setBookmarks}
        />
      </div>
    </section>
  );
}
