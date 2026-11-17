import { useId } from 'react';
import { Color } from '../../api/enums';
import type { Bookmark, Prefs } from '../../api/types';
import { bookmarkLabel } from '../../lib/catalog';
import { useSettings } from '../../stores/settings';
import controls from '../ui/controls.module.css';
import { Select } from '../ui/Select';
import styles from './ConnectionsDialog.module.css';

const COLORS: ReadonlyArray<{ value: Color; label: string }> = [
  { value: Color.ColorNone, label: 'None' },
  { value: Color.ColorRed, label: 'Red' },
  { value: Color.ColorOrange, label: 'Orange' },
  { value: Color.ColorGreen, label: 'Green' },
  { value: Color.ColorBlue, label: 'Blue' },
  { value: Color.ColorPurple, label: 'Purple' },
];

function showHiddenValue(value: boolean | null | undefined): string {
  return value == null ? '' : value ? 'show' : 'hide';
}

interface Props {
  prefs: Prefs;
  /** The draft's bookmarks: the only folders a connection can open at. */
  bookmarks: readonly Bookmark[];
  onChange: (patch: Partial<Prefs>) => void;
}

/** App-side settings every connection has, including Local and ~/.ssh/config hosts. */
export function ConnectionPrefsFields({ prefs, bookmarks, onChange }: Props) {
  const name = useId();
  const settings = useSettings((s) => s.settings);
  const globalLimit = settings?.globalTransfers ?? 1;
  const transfers = prefs.transfers ?? 0;
  // A limit saved before the global one was lowered still needs its own option.
  const limits = Array.from({ length: Math.max(globalLimit, transfers) }, (_, i) => i + 1);

  return (
    <div className={styles.fields}>
      <fieldset className={styles.fieldset}>
        <legend>Color tag</legend>
        <div className={styles.swatches}>
          {COLORS.map(({ value, label }) => (
            <input
              key={label}
              type="radio"
              name={`${name}-color`}
              className={styles.swatch}
              style={value === Color.ColorNone ? undefined : { background: `var(--tag-${value})` }}
              aria-label={label}
              title={label}
              checked={(prefs.color ?? Color.ColorNone) === value}
              onChange={() => onChange({ color: value })}
            />
          ))}
        </div>
      </fieldset>

      <label className={controls.label}>
        Open at
        <Select value={prefs.openAt ?? ''} onChange={(e) => onChange({ openAt: e.target.value })}>
          <option value="">Home folder</option>
          {bookmarks.map((b) => (
            <option key={b.path} value={b.path}>
              {bookmarkLabel(b)} — {b.path}
            </option>
          ))}
        </Select>
      </label>

      <div className={styles.pair}>
        <label className={controls.label}>
          Parallel transfers
          <Select
            value={String(transfers)}
            onChange={(e) => onChange({ transfers: Number(e.target.value) })}
          >
            <option value="0">
              {settings === null
                ? 'Use global setting'
                : `Use global setting (${settings.perConnectionTransfers})`}
            </option>
            {limits.map((n) => (
              <option key={n} value={n}>
                {n}
              </option>
            ))}
          </Select>
        </label>
        <label className={controls.label}>
          Hidden files
          <Select
            value={showHiddenValue(prefs.showHidden)}
            onChange={(e) =>
              onChange({ showHidden: e.target.value === '' ? null : e.target.value === 'show' })
            }
          >
            <option value="">Use global setting</option>
            <option value="show">Show</option>
            <option value="hide">Hide</option>
          </Select>
        </label>
      </div>
    </div>
  );
}
