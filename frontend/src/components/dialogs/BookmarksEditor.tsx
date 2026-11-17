import { useId, useState } from 'react';
import type { Bookmark } from '../../api/types';
import { bookmarkLabel } from '../../lib/catalog';
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

export function BookmarksEditor({ connId, bookmarks, openAt, onChange }: Props) {
  const listId = useId();
  const recents = useRecents((s) => (connId === null ? undefined : s.byConn[connId]));
  const [path, setPath] = useState('');
  const [removing, setRemoving] = useState<string | null>(null);

  if (connId === null) {
    return <p className={controls.hint}>Save this connection to bookmark its folders.</p>;
  }

  const patch = (at: number, change: Partial<Bookmark>) =>
    onChange(bookmarks.map((b, i) => (i === at ? { ...b, ...change } : b)));

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
      {bookmarks.map((b, i) => (
        <div key={b.path} className={styles.bookmark}>
          <button
            type="button"
            className={`${controls.iconButton} ${b.pinned ? styles.pinned : ''}`}
            aria-label={`${b.pinned ? 'Unpin' : 'Pin'} ${bookmarkLabel(b)}`}
            aria-pressed={b.pinned ?? false}
            title={b.pinned ? 'Unpin from favorites' : 'Pin as a favorite'}
            onClick={() => patch(i, { pinned: !b.pinned })}
          >
            <Icon name="star" fill={b.pinned ? 'currentColor' : 'none'} />
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
          {b.path === openAt && <span className={styles.badge}>Opens here</span>}
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
              aria-label={`Remove bookmark ${bookmarkLabel(b)}`}
              title="Remove bookmark"
              onClick={() => setRemoving(b.path)}
            >
              <Icon name="trash" />
            </button>
          )}
        </div>
      ))}
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
