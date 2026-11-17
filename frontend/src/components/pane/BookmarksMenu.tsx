import { useRef, useState } from 'react';
import { bookmarkAt, bookmarkLabel } from '../../lib/catalog';
import { IS_MAC } from '../../lib/platform';
import { useCatalog, useConnections } from '../../stores/connections';
import { canBookmark, openBookmark, toggleBookmark } from '../../stores/bookmarks';
import type { PaneId } from '../../stores/paneTypes';
import { usePanes } from '../../stores/panes';
import { useRecents } from '../../stores/recents';
import { useUi } from '../../stores/ui';
import controls from '../ui/controls.module.css';
import { Icon } from '../ui/Icon';
import { ContextMenu, type MenuItem } from './ContextMenu';

const SHORTCUT = IS_MAC ? '⌘D' : 'Ctrl+D';

/** The pane header's bookmark button and the menu it opens. */
export function BookmarksMenu({ paneId }: { paneId: PaneId }) {
  const connId = usePanes((s) => s.panes[paneId].connId);
  const path = usePanes((s) => s.panes[paneId].path);
  const lastLabel = usePanes((s) => s.panes[paneId].lastTarget?.label);
  const bookmarks = useConnections((s) => s.bookmarks);
  const recents = useRecents((s) => (connId === null ? undefined : s.byConn[connId]));
  const catalog = useCatalog();
  const [anchor, setAnchor] = useState<{ x: number; y: number } | null>(null);
  const button = useRef<HTMLButtonElement>(null);
  const wasOpen = useRef(false);

  const open = () => {
    const rect = button.current?.getBoundingClientRect();
    if (rect) setAnchor({ x: rect.left, y: rect.bottom + 2 });
  };

  const close = () => {
    setAnchor(null);
    button.current?.focus();
  };

  const items = (): MenuItem[] => {
    const here = connId === null ? undefined : catalog.find((c) => c.id === connId);
    const name = here?.name ?? lastLabel ?? 'this connection';
    const existing = connId === null ? undefined : bookmarkAt(bookmarks, connId, path);
    const allowed = canBookmark(connId);
    const connName = (id: string) => catalog.find((c) => c.id === id)?.name;
    const manage = () =>
      useUi.getState().openDialog({ kind: 'connections', ...(here && { connId: here.id }) });

    const list: MenuItem[] = [
      {
        label: existing ? 'Remove bookmark' : 'Bookmark this folder',
        shortcut: SHORTCUT,
        disabled: !allowed,
        ...(!allowed && { hint: 'Save this host to bookmark its folders' }),
        onSelect: () => void toggleBookmark(paneId),
      },
    ];

    const section = (heading: string, entries: MenuItem[]) => {
      const [first, ...rest] = entries;
      if (first) list.push({ ...first, heading }, ...rest);
    };

    section(
      'Favourites',
      bookmarks
        .filter((b) => b.pinned && connName(b.connId) !== undefined)
        .map((b) => ({
          label: bookmarkLabel(b),
          detail: connName(b.connId) ?? '',
          onSelect: () => void openBookmark(paneId, b),
        }))
    );
    section(
      `Bookmarks on ${name}`,
      bookmarks
        .filter((b) => b.connId === connId)
        .map((b) => ({
          label: bookmarkLabel(b),
          detail: b.path,
          onSelect: () => void openBookmark(paneId, b),
        }))
    );
    section(
      'Recent folders',
      (recents ?? [])
        .filter((p) => p !== path)
        .map((p) => ({
          label: p,
          onSelect: () => usePanes.getState().navigate(paneId, p),
        }))
    );
    list.push({ label: 'Manage bookmarks…', separator: true, onSelect: manage });
    return list;
  };

  return (
    <>
      <button
        ref={button}
        type="button"
        className={controls.iconButton}
        aria-label="Bookmarks"
        aria-haspopup="menu"
        aria-expanded={anchor !== null}
        title={`Bookmarks (${SHORTCUT} bookmarks this folder)`}
        disabled={connId === null}
        onPointerDown={() => {
          wasOpen.current = anchor !== null;
        }}
        onClick={() => {
          // The press already closed an open menu; don't reopen it.
          if (wasOpen.current) wasOpen.current = false;
          else open();
        }}
      >
        <Icon name="bookmark" />
      </button>
      {anchor && <ContextMenu x={anchor.x} y={anchor.y} items={items()} onClose={close} />}
    </>
  );
}
