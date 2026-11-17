import type { HostFields } from '../../lib/connectionDraft';
import controls from '../ui/controls.module.css';
import styles from './ConnectionsDialog.module.css';

interface Props {
  fields: HostFields;
  onChange: (next: HostFields) => void;
}

/** The editable address of a host saved in Portunus. */
export function SavedHostFields({ fields, onChange }: Props) {
  const set = <K extends keyof HostFields>(key: K, value: HostFields[K]) =>
    onChange({ ...fields, [key]: value });

  return (
    <div className={styles.fields}>
      <label className={controls.label}>
        Name
        <input
          className={controls.field}
          value={fields.name}
          autoFocus={fields.id === ''}
          onChange={(e) => set('name', e.target.value)}
        />
      </label>
      <div className={styles.pair}>
        <label className={controls.label}>
          Host
          <input
            className={controls.field}
            value={fields.host}
            placeholder="example.com"
            spellCheck={false}
            autoCapitalize="off"
            onChange={(e) => set('host', e.target.value)}
          />
        </label>
        <label className={controls.label}>
          Port
          <input
            className={controls.field}
            value={fields.port}
            placeholder="22"
            inputMode="numeric"
            spellCheck={false}
            onChange={(e) => set('port', e.target.value)}
          />
        </label>
      </div>
      <label className={controls.label}>
        User
        <input
          className={controls.field}
          value={fields.user}
          placeholder="Your login name"
          spellCheck={false}
          autoCapitalize="off"
          autoComplete="off"
          onChange={(e) => set('user', e.target.value)}
        />
      </label>
    </div>
  );
}
