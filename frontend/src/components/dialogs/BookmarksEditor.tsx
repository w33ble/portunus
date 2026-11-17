import { type PointerEvent as ReactPointerEvent, useEffect, useId, useRef, useState } from 'react';
import type { Bookmark } from '../../api/types';
import { bookmarkLabel } from '../../lib/catalog';
import { moveBookmark } from '../../lib/connectionDraft';
import { normalizePath } from '../../lib/paths';
import { useRecents } from '../../stores/recents';
import controls from '../ui/controls.module.css';
import { Icon } from '../ui/Icon';
import styles from './ConnectionsDialog.module.css';

const ABSOLUTE = /^(\/|[A-Za-z]:\/)/;

interface Props {
  /** Null while the connection isn't saved: bookmarks need a connection to belong to. */
  connId: string | null;
  bookmarks: readonly Bookmark[];
  /** The folder the connection opens at, if any. */
  openAt: string | undefined;
  onChange: (bookmarks: Bookmark[]) => void;
}

/** A row being dragged: where it started and where it would land. */
interface Drag {
  from: number;
  to: number;
}

export function BookmarksEditor({ connId, bookmarks, openAt, onChange }: Props) {
  const listId = useId();
  const recents = useRecents((s) => (connId === null ? undefined : s.byConn[connId]));
  const [path, setPath] = useState('');
  const [removing, setRemoving] = useState<string | null>(null);
  const [drag, setDrag] = useState<Drag | null>(null);
  const rows = useRef(new Map<string, HTMLElement>());
  // Where each row's midpoint was when the drag began; the landing spot is read against these.
  const mids = useRef<number[]>([]);
  const landing = useRef<Drag | null>(null);
  const refocus = useRef<string | null>(null);
  const dragging = drag !== null;

  // Moving a focused row in the DOM drops its focus; hand it back to the grip.
  useEffect(() => {
    if (refocus.current === null) return;
    rows.current.get(refocus.current)?.querySelector('button')?.focus();
    refocus.current = null;
  });

  useEffect(() => {
    if (!dragging) return;
    const end = (commit: boolean) => {
      const done = landing.current;
      landing.current = null;
      setDrag(null);
      if (commit && done && done.from !== done.to) {
        onChange(moveBookmark(bookmarks, done.from, done.to));
      }
    };
    const move = (e: PointerEvent) => {
      const from = landing.current?.from ?? 0;
      const to = mids.current.filter((m, j) => j !== from && m < e.clientY).length;
      landing.current = { from, to };
      setDrag({ from, to });
    };
    const up = () => end(true);
    const key = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault();
        e.stopPropagation();
        end(false);
      }
    };
    window.addEventListener('pointermove', move);
    window.addEventListener('pointerup', up);
    window.addEventListener('pointercancel', up);
    window.addEventListener('keydown', key, true);
    return () => {
      window.removeEventListener('pointermove', move);
      window.removeEventListener('pointerup', up);
      window.removeEventListener('pointercancel', up);
      window.removeEventListener('keydown', key, true);
    };
  }, [dragging, bookmarks, onChange]);

  if (connId === null) {
    return <p className={controls.hint}>Save this connection to bookmark its folders.</p>;
  }

  const patch = (at: number, change: Partial<Bookmark>) =>
    onChange(bookmarks.map((b, i) => (i === at ? { ...b, ...change } : b)));

  const startDrag = (e: ReactPointerEvent<HTMLElement>, from: number) => {
    if (e.button !== 0) return;
    e.preventDefault();
    e.currentTarget.focus();
    mids.current = bookmarks.map((b) => {
      const r = rows.current.get(b.path)?.getBoundingClientRect();
      return r ? r.top + r.height / 2 : 0;
    });
    setRemoving(null);
    landing.current = { from, to: from };
    setDrag({ from, to: from });
  };

  const shown = drag ? moveBookmark(bookmarks, drag.from, drag.to) : bookmarks;
  const target = normalizePath(path);
  const addable =
    path.trim() !== '' && ABSOLUTE.test(target) && !bookmarks.some((b) => b.path === target);
  const add = () => {
    if (!addable) return;
    onChange([...bookmarks, { id: '', connId, path: target }]);
    setPath('');
  };
  const suggestions = (recents ?? []).filter((p) => !bookmarks.some((b) => b.path === p));

  return (
    <div className={styles.bookmarks}>
      {bookmarks.length === 0 && <p className={controls.hint}>No bookmarks on this connection.</p>}
      {shown.map((b) => {
        const i = bookmarks.indexOf(b);
        const label = bookmarkLabel(b);
        return (
          <div
            key={b.path}
            ref={(el) => {
              if (el) rows.current.set(b.path, el);
              else rows.current.delete(b.path);
            }}
            className={styles.bookmark}
            data-dragging={drag?.from === i}
          >
            <button
              type="button"
              className={`${controls.iconButton} ${styles.grip}`}
              aria-label={`Move ${label}`}
              title="Drag to reorder, or use the arrow keys"
              onPointerDown={(e) => startDrag(e, i)}
              onKeyDown={(e) => {
                if (e.key !== 'ArrowUp' && e.key !== 'ArrowDown') return;
                e.preventDefault();
                const to = i + (e.key === 'ArrowUp' ? -1 : 1);
                if (to < 0 || to >= bookmarks.length) return;
                refocus.current = b.path;
                onChange(moveBookmark(bookmarks, i, to));
              }}
            >
              <Icon name="grip" />
            </button>
            <div className={styles.bookmarkText}>
              <input
                className={controls.field}
                aria-label={`Name for ${b.path}`}
                placeholder={bookmarkLabel({ ...b, name: '' })}
                value={b.name ?? ''}
                onChange={(e) => patch(i, { name: e.target.value })}
              />
              <span className={`${controls.mono} ${styles.path}`}>{b.path}</span>
            </div>
            {b.path === openAt && (
              <span
                className={styles.opensHere}
                role="img"
                aria-label="Opens here"
                title="Opens here: new connections start in this folder"
              >
                <Icon name="home" />
              </span>
            )}
            <button
              type="button"
              className={`${controls.iconButton} ${b.pinned ? styles.pinned : ''}`}
              aria-label={`${b.pinned ? 'Unpin' : 'Pin'} ${label}`}
              aria-pressed={b.pinned ?? false}
              title={b.pinned ? 'Unpin from favorites' : 'Pin as a favorite'}
              onClick={() => patch(i, { pinned: !b.pinned })}
            >
              <Icon name="star" fill={b.pinned ? 'currentColor' : 'none'} />
            </button>
            {removing === b.path ? (
              <div className={styles.confirmRow}>
                <button
                  type="button"
                  className={`${controls.button} ${controls.danger}`}
                  onClick={() => {
                    setRemoving(null);
                    onChange(bookmarks.filter((_, j) => j !== i));
                  }}
                >
                  Remove
                </button>
                <button type="button" className={controls.button} onClick={() => setRemoving(null)}>
                  Keep
                </button>
              </div>
            ) : (
              <button
                type="button"
                className={controls.iconButton}
                aria-label={`Remove bookmark ${label}`}
                title="Remove bookmark"
                onClick={() => setRemoving(b.path)}
              >
                <Icon name="trash" />
              </button>
            )}
          </div>
        );
      })}
      <div className={styles.addBookmark}>
        <input
          className={controls.field}
          aria-label="Folder to bookmark"
          placeholder="Add a folder, e.g. /srv/logs"
          list={listId}
          spellCheck={false}
          autoCapitalize="off"
          value={path}
          onChange={(e) => setPath(e.target.value)}
          onKeyDown={(e) => {
            if (e.key !== 'Enter') return;
            e.preventDefault();
            add();
          }}
        />
        <datalist id={listId}>
          {suggestions.map((p) => (
            <option key={p} value={p} />
          ))}
        </datalist>
        <button type="button" className={controls.button} disabled={!addable} onClick={add}>
          Add
        </button>
      </div>
    </div>
  );
}
