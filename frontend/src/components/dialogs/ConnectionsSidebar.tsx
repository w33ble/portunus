import { type KeyboardEvent, useEffect, useId, useRef, useState } from 'react';
import type { KnownConnection } from '../../lib/catalog';
import { NEW_HOST_KEY, NEW_S3_KEY } from '../../lib/connectionDraft';
import controls from '../ui/controls.module.css';
import { Icon } from '../ui/Icon';
import styles from './ConnectionsDialog.module.css';
import type { ConnectionEditor } from './useConnectionEditor';

interface Row {
  id: string;
  name: string;
  color: string;
  hidden: boolean;
  favorite: boolean;
}

interface Group {
  id: string;
  label: string;
  rows: Row[];
}

function groupsOf(editor: ConnectionEditor, query: string): Group[] {
  const { catalog, key, draft, label } = editor;
  const toRow = (c: KnownConnection): Row => {
    const own = c.id === key;
    const prefs = own ? draft.prefs : c.prefs;
    return {
      id: c.id,
      name: own ? label : c.name,
      color: prefs.color ?? '',
      hidden: c.source !== 'local' && (prefs.hideInPicker ?? false),
      favorite: prefs.favorite ?? false,
    };
  };
  const needle = query.trim().toLowerCase();
  const rowsOf = (test: (c: KnownConnection) => boolean, fresh?: string): Row[] => {
    const rows = catalog
      .filter(test)
      .map(toRow)
      .filter((r) => needle === '' || r.id === key || r.name.toLowerCase().includes(needle));
    if (fresh !== undefined && key === fresh) {
      rows.push({
        id: fresh,
        name: label,
        color: draft.prefs.color ?? '',
        hidden: false,
        favorite: draft.prefs.favorite ?? false,
      });
    }
    return rows;
  };
  return [
    { id: 'local', label: 'Local', rows: rowsOf((c) => c.source === 'local') },
    {
      id: 'ssh',
      label: 'SSH',
      rows: rowsOf((c) => c.source === 'config' || c.source === 'saved', NEW_HOST_KEY),
    },
    { id: 's3', label: 'S3', rows: rowsOf((c) => c.source === 's3', NEW_S3_KEY) },
  ].filter((g) => g.rows.length > 0);
}

/** Add and remove under the list; each asks for confirmation inline, in place of the buttons. */
function ListActions({ editor }: { editor: ConnectionEditor }) {
  const [mode, setMode] = useState<'idle' | 'adding' | 'removing'>('idle');
  const { conn, busy } = editor;
  const isNew = conn === null;
  const removable = isNew || conn.source === 'saved' || conn.source === 's3';

  if (mode === 'adding') {
    const add = (kind: 'host' | 's3') => {
      setMode('idle');
      editor.add(kind);
    };
    return (
      <div className={styles.listActions}>
        <button type="button" className={controls.button} onClick={() => add('host')}>
          SSH host
        </button>
        <button type="button" className={controls.button} onClick={() => add('s3')}>
          S3 profile
        </button>
        <button
          type="button"
          className={controls.iconButton}
          aria-label="Cancel adding"
          title="Cancel"
          onClick={() => setMode('idle')}
        >
          <Icon name="close" />
        </button>
      </div>
    );
  }

  if (mode === 'removing') {
    return (
      <div className={styles.listActions}>
        <span className={styles.confirmText}>Remove “{editor.label}”?</span>
        <button
          type="button"
          className={`${controls.button} ${controls.danger}`}
          disabled={busy}
          onClick={() => void editor.remove()}
        >
          Remove
        </button>
        <button type="button" className={controls.button} onClick={() => setMode('idle')}>
          Keep
        </button>
      </div>
    );
  }

  return (
    <div className={styles.listActions}>
      <button
        type="button"
        className={controls.iconButton}
        aria-label="Add connection"
        title="Add connection"
        onClick={() => setMode('adding')}
      >
        <Icon name="plus" />
      </button>
      <button
        type="button"
        className={controls.iconButton}
        aria-label="Remove connection"
        title={removable ? 'Remove connection' : 'This connection can’t be removed here'}
        disabled={!removable || busy}
        onClick={() => (isNew ? void editor.remove() : setMode('removing'))}
      >
        <Icon name="minus" />
      </button>
    </div>
  );
}

export function ConnectionsSidebar({ editor }: { editor: ConnectionEditor }) {
  const [query, setQuery] = useState('');
  const baseId = useId();
  const focusAfter = useRef<string | null>(null);
  const groups = groupsOf(editor, query);
  const ids = groups.flatMap((g) => g.rows.map((r) => r.id));

  // After a keyboard move, focus follows the selection once it has actually changed.
  useEffect(() => {
    if (focusAfter.current !== editor.key) return;
    focusAfter.current = null;
    const el = document.getElementById(`${baseId}-${editor.key}`);
    el?.focus();
    el?.scrollIntoView?.({ block: 'nearest' });
  }, [editor.key, baseId]);

  const onKeyDown = (event: KeyboardEvent) => {
    const at = ids.indexOf(editor.key);
    const target =
      event.key === 'ArrowDown'
        ? ids[Math.min(at + 1, ids.length - 1)]
        : event.key === 'ArrowUp'
          ? ids[Math.max(at - 1, 0)]
          : event.key === 'Home'
            ? ids[0]
            : event.key === 'End'
              ? ids.at(-1)
              : undefined;
    if (target === undefined) return;
    event.preventDefault();
    if (target === editor.key) return;
    focusAfter.current = target;
    editor.select(target);
  };

  return (
    <div className={styles.sidebar}>
      <input
        className={controls.field}
        type="search"
        aria-label="Filter connections"
        placeholder="Filter"
        spellCheck={false}
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === 'Enter') e.preventDefault();
        }}
      />
      <div
        role="listbox"
        aria-label="Connections"
        className={styles.list}
        onKeyDown={onKeyDown}
        onPointerDown={() => {
          focusAfter.current = null;
        }}
      >
        {groups.length === 0 && <p className={controls.hint}>No connections match.</p>}
        {groups.map((g) => (
          <div key={g.id} role="group" aria-labelledby={`${baseId}-group-${g.id}`}>
            <div id={`${baseId}-group-${g.id}`} className={styles.groupLabel}>
              {g.label}
            </div>
            {g.rows.map((r) => {
              const selected = r.id === editor.key;
              return (
                <div
                  key={r.id}
                  id={`${baseId}-${r.id}`}
                  role="option"
                  aria-selected={selected}
                  tabIndex={selected ? 0 : -1}
                  className={styles.row}
                  data-selected={selected}
                  data-hidden={r.hidden}
                  onClick={() => editor.select(r.id)}
                >
                  <span
                    className={styles.dot}
                    title={r.color === '' ? undefined : `Tagged ${r.color}`}
                    style={r.color === '' ? undefined : { background: `var(--tag-${r.color})` }}
                  />
                  <span className={styles.rowName}>{r.name}</span>
                  {selected && editor.dirty && <span className={styles.edited}>· edited</span>}
                  {r.favorite && (
                    <span className={styles.favoriteMark} aria-hidden="true">
                      <Icon name="star" size={14} fill="currentColor" />
                    </span>
                  )}
                  {r.hidden && (
                    <span className={styles.hiddenMark} title="Hidden from the picker">
                      <Icon name="eyeOff" size={14} />
                    </span>
                  )}
                </div>
              );
            })}
          </div>
        ))}
      </div>
      <ListActions key={editor.key} editor={editor} />
    </div>
  );
}
