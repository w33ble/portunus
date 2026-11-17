import type { ReactNode } from 'react';
import { SYSTEM_THEME, THEMES } from '../../lib/theme';
import { useSettings } from '../../stores/settings';
import styles from './ThemePicker.module.css';

/** A miniature window drawn with the theme's own tokens: chrome, text, a selected row, code, accent. */
function Thumbnail({ theme, className }: { theme: string; className?: string }) {
  return (
    <span className={`${styles.thumb} ${className ?? ''}`} data-theme={theme} aria-hidden="true">
      <span className={styles.bar} />
      <span className={styles.body}>
        <span className={styles.line} />
        <span className={styles.selected}>
          <span className={styles.selectedLine} />
        </span>
        <span className={styles.code}>
          <span className={styles.keyword} />
          <span className={styles.string} />
          <span className={styles.title} />
        </span>
        <span className={styles.secondary} />
        <span className={styles.accent} />
      </span>
    </span>
  );
}

interface OptionProps {
  id: string;
  name: string;
  hint?: string;
  checked: boolean;
  onChoose: (id: string) => void;
  className?: string;
  children: ReactNode;
}

function Option({ id, name, hint, checked, onChoose, className, children }: OptionProps) {
  return (
    <label className={`${styles.option} ${className ?? ''}`}>
      <input
        type="radio"
        name="theme"
        value={id}
        checked={checked}
        onChange={() => onChoose(id)}
        className={styles.radio}
      />
      {children}
      <span className={styles.text}>
        <span>{name}</span>
        {hint !== undefined && <span className={styles.hint}>{hint}</span>}
      </span>
    </label>
  );
}

/** Every built-in theme as a card; picking one applies and saves it at once. */
export function ThemePicker() {
  const current = useSettings((s) => s.settings?.theme);
  const save = useSettings((s) => s.save);
  if (current === undefined) return null;

  const choose = (theme: string) => void save({ theme });

  return (
    <div role="radiogroup" aria-label="Theme" className={styles.grid}>
      <Option
        id={SYSTEM_THEME}
        name="System"
        hint="Portunus Light or Dark, following the system appearance"
        checked={current === SYSTEM_THEME}
        onChoose={choose}
        className={styles.wide}
      >
        <span className={styles.split} aria-hidden="true">
          <Thumbnail theme="light" />
          <Thumbnail theme="dark" className={styles.splitDark} />
        </span>
      </Option>
      {THEMES.map((t) => (
        <Option key={t.id} id={t.id} name={t.name} checked={current === t.id} onChoose={choose}>
          <Thumbnail theme={t.id} />
        </Option>
      ))}
    </div>
  );
}
