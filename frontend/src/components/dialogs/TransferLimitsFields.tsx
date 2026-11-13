import { useState } from 'react';
import { useSettings } from '../../stores/settings';
import controls from '../ui/controls.module.css';
import styles from './TransferLimitsFields.module.css';

const MAX = 32;

interface FieldProps {
  label: string;
  value: number;
  onCommit: (value: number) => void;
}

function NumberField({ label, value, onCommit }: FieldProps) {
  // null while not editing, so the field always mirrors the saved value.
  const [draft, setDraft] = useState<string | null>(null);

  const commit = () => {
    const parsed = Math.round(Number(draft ?? value));
    const next = Number.isFinite(parsed) ? Math.min(Math.max(parsed, 1), MAX) : value;
    setDraft(null);
    if (next !== value) onCommit(next);
  };

  return (
    <label className={styles.field}>
      <span>{label}</span>
      <input
        className={`${controls.field} ${styles.number}`}
        type="number"
        min={1}
        max={MAX}
        inputMode="numeric"
        value={draft ?? String(value)}
        onChange={(e) => setDraft(e.target.value)}
        onBlur={commit}
        onKeyDown={(e) => {
          if (e.key === 'Enter') {
            e.preventDefault();
            commit();
          }
        }}
      />
    </label>
  );
}

/** Global and per-connection parallel transfer limits, saved through SettingsService. */
export function TransferLimitsFields() {
  const settings = useSettings((s) => s.settings);
  const save = useSettings((s) => s.save);
  if (settings === null) return null;

  return (
    <div className={styles.row}>
      <NumberField
        label="Parallel transfers"
        value={settings.globalTransfers}
        onCommit={(globalTransfers) =>
          void save({
            globalTransfers,
            perConnectionTransfers: Math.min(settings.perConnectionTransfers, globalTransfers),
          })
        }
      />
      <NumberField
        label="Per connection"
        value={settings.perConnectionTransfers}
        onCommit={(perConnectionTransfers) =>
          void save({
            perConnectionTransfers,
            globalTransfers: Math.max(settings.globalTransfers, perConnectionTransfers),
          })
        }
      />
    </div>
  );
}
