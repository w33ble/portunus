import styles from './ConnectionPicker.module.css';

/** A connection's color tag: a non-text mark, hidden from assistive tech. */
export function ColorDot({ color }: { color: string }) {
  return (
    <span
      className={styles.dot}
      style={{ backgroundColor: `var(--tag-${color})` }}
      title={`${color} tag`}
      aria-hidden="true"
    />
  );
}
