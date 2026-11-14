import { type Ref, useEffect, useImperativeHandle, useRef, useState } from 'react';
import { breadcrumbs } from '../../lib/paths';
import styles from './PathBar.module.css';

function scrollToEnd(el: HTMLDivElement | null): void {
  if (el) el.scrollLeft = el.scrollWidth;
}

export interface PathBarHandle {
  /** Starts editing: selects the current path, or starts from `prefill` with the caret after it. */
  edit: (prefill?: string) => void;
}

interface Props {
  path: string;
  disabled: boolean;
  onNavigate: (path: string) => void;
  ref?: Ref<PathBarHandle>;
}

/** Clickable breadcrumb that turns into a text field on ⌘L or a click in the empty space. */
export function PathBar({ path, disabled, onNavigate, ref }: Props) {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(path);
  const input = useRef<HTMLInputElement>(null);
  const prefilled = useRef(false);

  useImperativeHandle(ref, () => ({
    edit(prefill) {
      prefilled.current = prefill !== undefined;
      setDraft(prefill ?? path);
      setEditing(true);
    },
  }));

  useEffect(() => {
    const el = input.current;
    if (!editing || !el) return;
    if (prefilled.current) el.setSelectionRange(el.value.length, el.value.length);
    else el.select();
  }, [editing]);

  if (editing) {
    const finish = (commit: boolean) => {
      setEditing(false);
      if (commit && draft.trim() !== '' && draft.trim() !== path) onNavigate(draft);
    };
    return (
      <input
        ref={input}
        className={styles.input}
        value={draft}
        aria-label="Path"
        spellCheck={false}
        autoCapitalize="off"
        autoCorrect="off"
        autoFocus
        onChange={(e) => setDraft(e.target.value)}
        onBlur={() => setEditing(false)}
        onKeyDown={(e) => {
          if (e.key === 'Enter') {
            e.preventDefault();
            finish(true);
          } else if (e.key === 'Escape') {
            e.preventDefault();
            e.stopPropagation();
            finish(false);
          }
        }}
      />
    );
  }

  const crumbs = breadcrumbs(path);
  return (
    <nav className={styles.bar} aria-label="Path">
      <div key={path} ref={scrollToEnd} className={styles.crumbs}>
        {crumbs.map((crumb, i) => (
          <button
            key={crumb.path}
            type="button"
            className={styles.crumb}
            data-current={i === crumbs.length - 1}
            disabled={disabled}
            onClick={() => onNavigate(crumb.path)}
          >
            {crumb.label}
          </button>
        ))}
        <button
          type="button"
          className={styles.filler}
          aria-label="Edit path"
          tabIndex={-1}
          disabled={disabled}
          onClick={() => {
            prefilled.current = false;
            setDraft(path);
            setEditing(true);
          }}
        />
      </div>
    </nav>
  );
}
