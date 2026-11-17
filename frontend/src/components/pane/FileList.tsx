import { useVirtualizer } from '@tanstack/react-virtual';
import { type MouseEvent, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { Entry } from '../../api/types';
import { ENTRIES_MIME, encodeDragPayload } from '../../lib/dragData';
import { shownEntries } from '../../lib/listing';
import { EMPTY_TYPE_AHEAD, TYPE_AHEAD_RESET_MS, filterEntries } from '../../lib/match';
import { clickSelect, pruneSelection, selectOnly } from '../../lib/selection';
import { IS_MAC } from '../../lib/platform';
import { visibleEntries } from '../../lib/sort';
import {
  copyToOtherPane,
  moveToOtherPane,
  openEntry,
  requestDelete,
  requestNewFolder,
  requestRename,
  selectedEntries,
} from '../../stores/fileOps';
import { bookmarkFolders, canBookmark, toggleBookmark } from '../../stores/bookmarks';
import type { PaneId } from '../../stores/paneTypes';
import { bookmarkAt } from '../../lib/catalog';
import { usePanes } from '../../stores/panes';
import { useConnections, useShowHidden } from '../../stores/connections';
import { useUi } from '../../stores/ui';
import { ColumnHeader } from './ColumnHeader';
import { ContextMenu, type MenuItem } from './ContextMenu';
import { FileRow } from './FileRow';
import { FilterBar } from './FilterBar';
import styles from './FileList.module.css';
import { useFileListKeys } from './useFileListKeys';
import { useListDrop } from './useListDrop';

const ROW_HEIGHT = 22;
const HEADER_HEIGHT = 22;

interface MenuState {
  x: number;
  y: number;
  /** Opened on a row, as opposed to the empty space below the rows. */
  onRow: boolean;
}

const BOOKMARK_HINT = 'Save this host to bookmark its folders';
const MOD = IS_MAC ? '⌘' : 'Ctrl+';

interface Props {
  paneId: PaneId;
  /** Opens the pane's path editor with `prefill` typed in. */
  onEditPath: (prefill: string) => void;
}

export function FileList({ paneId, onEditPath }: Props) {
  const pane = usePanes((s) => s.panes[paneId]);
  const showHidden = useShowHidden(pane.connId);
  const scrollerRef = useRef<HTMLDivElement>(null);
  const typeAhead = useRef(EMPTY_TYPE_AHEAD);
  const [typed, setTyped] = useState('');
  const [menu, setMenu] = useState<MenuState | null>(null);
  const bookmarks = useConnections((s) => s.bookmarks);
  const drop = useListDrop(paneId, pane.path);

  const shown = shownEntries(pane.listing);
  const unfiltered = useMemo(
    () => visibleEntries(shown, showHidden, pane.sort),
    [shown, showHidden, pane.sort]
  );
  const query = pane.filter ?? '';
  const visible = useMemo(() => filterEntries(unfiltered, query), [unfiltered, query]);
  const { selection } = pane;

  // Keyboard shortcuts work from launch: the active pane's list starts focused.
  useEffect(() => {
    if (useUi.getState().activePane === paneId) scrollerRef.current?.focus({ preventScroll: true });
  }, [paneId]);

  // The typed-prefix label disappears when type-ahead resets.
  useEffect(() => {
    if (typed === '') return;
    const timer = setTimeout(() => setTyped(''), TYPE_AHEAD_RESET_MS);
    return () => clearTimeout(timer);
  }, [typed]);

  // Drop selections that left the list (refresh, delete, hidden toggled, filtered out).
  useEffect(() => {
    const pruned = pruneSelection(
      selection,
      visible.map((e) => e.path)
    );
    if (pruned !== selection) usePanes.getState().setSelection(paneId, pruned);
  }, [visible, selection, paneId]);

  // TanStack Virtual returns functions React Compiler can't memoize; nothing here is compiled.
  // oxlint-disable-next-line react/incompatible-library
  const virtualizer = useVirtualizer({
    count: visible.length,
    getScrollElement: () => scrollerRef.current,
    estimateSize: () => ROW_HEIGHT,
    scrollMargin: HEADER_HEIGHT,
    overscan: 12,
    getItemKey: (i) => visible[i]?.path ?? i,
  });

  const cursorIndex = useMemo(
    () => (selection.cursor === null ? -1 : visible.findIndex((e) => e.path === selection.cursor)),
    [visible, selection.cursor]
  );
  useEffect(() => {
    if (cursorIndex >= 0) virtualizer.scrollToIndex(cursorIndex, { align: 'auto' });
  }, [cursorIndex, virtualizer]);

  const pageRows = Math.max(
    1,
    Math.floor(((scrollerRef.current?.clientHeight ?? 400) - HEADER_HEIGHT) / ROW_HEIGHT) - 1
  );
  const onKeyDown = useFileListKeys({
    paneId,
    visible,
    selection,
    pageRows,
    filtering: pane.filter !== null,
    typeAhead,
    onTyped: setTyped,
    onEditPath,
  });

  const paths = useMemo(() => visible.map((e) => e.path), [visible]);

  const onClick = useCallback(
    (entry: Entry, event: MouseEvent) => {
      const panes = usePanes.getState();
      const current = panes.panes[paneId].selection;
      panes.setSelection(
        paneId,
        clickSelect(current, paths, entry.path, {
          toggle: event.metaKey || event.ctrlKey,
          shift: event.shiftKey,
        })
      );
    },
    [paneId, paths]
  );

  const onContextMenu = useCallback(
    (entry: Entry | null, event: MouseEvent) => {
      event.preventDefault();
      event.stopPropagation();
      if (entry !== null) {
        const current = usePanes.getState().panes[paneId].selection;
        if (!current.selected.has(entry.path)) {
          usePanes.getState().setSelection(paneId, selectOnly(entry.path));
        }
      }
      setMenu({ x: event.clientX, y: event.clientY, onRow: entry !== null });
    },
    [paneId]
  );

  const onRowContextMenu = useCallback(
    (entry: Entry, event: MouseEvent) => onContextMenu(entry, event),
    [onContextMenu]
  );
  const onOpen = useCallback((entry: Entry) => openEntry(paneId, entry), [paneId]);

  const onDragStart = useCallback(
    (entry: Entry, event: React.DragEvent) => {
      const panes = usePanes.getState();
      let sel = panes.panes[paneId].selection;
      if (!sel.selected.has(entry.path)) {
        sel = selectOnly(entry.path);
        panes.setSelection(paneId, sel);
      }
      const connId = panes.panes[paneId].connId;
      if (connId === null) {
        event.preventDefault();
        return;
      }
      const dragged = paths.filter((p) => sel.selected.has(p));
      event.dataTransfer.setData(ENTRIES_MIME, encodeDragPayload({ connId, paths: dragged }));
      event.dataTransfer.effectAllowed = 'copyMove';
    },
    [paneId, paths]
  );

  const menuItems = (): MenuItem[] => {
    const picked = selectedEntries(paneId);
    const [first] = picked;
    const any = picked.length > 0;
    return [
      {
        label: 'Open',
        disabled: picked.length !== 1 || !first?.isDir,
        shortcut: `${MOD}↓`,
        onSelect: () => first && openEntry(paneId, first),
      },
      {
        label: 'Copy to other pane',
        disabled: !any,
        shortcut: 'F5',
        onSelect: () => copyToOtherPane(paneId),
      },
      {
        label: 'Move to other pane',
        disabled: !any,
        shortcut: 'F6',
        onSelect: () => moveToOtherPane(paneId),
      },
      {
        label: 'Rename…',
        disabled: picked.length !== 1,
        shortcut: 'F2',
        onSelect: () => requestRename(paneId),
      },
      {
        label: 'Delete…',
        disabled: !any,
        shortcut: `${MOD}⌫`,
        onSelect: () => requestDelete(paneId),
      },
      ...bookmarkItems(menu?.onRow ? picked.filter((e) => e.isDir).map((e) => e.path) : null),
      {
        label: 'New folder…',
        separator: true,
        shortcut: `${MOD}⇧N`,
        onSelect: () => requestNewFolder(paneId),
      },
      {
        label: 'Refresh',
        shortcut: `${MOD}R`,
        onSelect: () => usePanes.getState().refresh(paneId),
      },
    ];
  };

  // `folders` is the selected folders of a row menu, or null for the empty-space menu.
  const bookmarkItems = (folders: string[] | null): MenuItem[] => {
    if (folders?.length === 0) return [];
    const hint = canBookmark(pane.connId) ? {} : { disabled: true, hint: BOOKMARK_HINT };
    const marked = (path: string) =>
      pane.connId !== null && bookmarkAt(bookmarks, pane.connId, path) !== undefined;
    if (folders === null || folders.length === 1) {
      const path = folders?.[0] ?? pane.path;
      const label = marked(path)
        ? 'Remove bookmark…'
        : folders === null
          ? 'Bookmark this folder'
          : 'Bookmark folder';
      return [{ label, ...hint, onSelect: () => void toggleBookmark(paneId, path) }];
    }
    const fresh = folders.filter((p) => !marked(p));
    const label = fresh.length === 1 ? 'Bookmark folder' : `Bookmark ${fresh.length} folders`;
    return [
      {
        label,
        ...(fresh.length === 0 ? { disabled: true } : hint),
        onSelect: () => void bookmarkFolders(paneId, fresh),
      },
    ];
  };

  const loading = pane.status.kind === 'loading' || pane.status.kind === 'connecting';
  const items = virtualizer.getVirtualItems();

  const leaveFilter = (pickBest: boolean) => {
    const [best] = visible;
    if (pickBest && best) usePanes.getState().setSelection(paneId, selectOnly(best.path));
    scrollerRef.current?.focus({ preventScroll: true });
  };

  return (
    <div
      className={styles.root}
      data-drop={drop.target?.kind === 'list' ? 'list' : 'none'}
      {...drop.listProps}
    >
      {pane.filter !== null && (
        <FilterBar
          paneId={paneId}
          query={pane.filter}
          matches={visible.length}
          total={unfiltered.length}
          onLeave={leaveFilter}
        />
      )}
      <div
        ref={scrollerRef}
        className={styles.scroller}
        role="listbox"
        aria-multiselectable="true"
        aria-label={`Files in ${pane.path}`}
        aria-busy={loading}
        tabIndex={0}
        data-filelist=""
        onKeyDown={onKeyDown}
        onContextMenu={(e) => onContextMenu(null, e)}
        onClick={(e) => {
          if (e.target === e.currentTarget) {
            usePanes
              .getState()
              .setSelection(paneId, { ...selection, selected: new Set(), anchor: null });
          }
        }}
      >
        <ColumnHeader
          sort={pane.sort}
          onSort={(key) => usePanes.getState().toggleSort(paneId, key)}
        />
        <div className={styles.body} style={{ height: virtualizer.getTotalSize() }}>
          {items.map((item) => {
            const entry = visible[item.index];
            if (!entry) return null;
            return (
              <FileRow
                key={item.key}
                paneId={paneId}
                entry={entry}
                index={item.index}
                top={item.start - HEADER_HEIGHT}
                selected={selection.selected.has(entry.path)}
                cursor={selection.cursor === entry.path}
                query={query}
                dropTarget={drop.target?.kind === 'row' && drop.target.path === entry.path}
                onClick={onClick}
                onOpen={onOpen}
                onContextMenu={onRowContextMenu}
                onDragStart={onDragStart}
                onDragOver={drop.onRowDragOver}
                onDragLeave={drop.onRowDragLeave}
                onDrop={drop.onRowDrop}
              />
            );
          })}
        </div>
        {visible.length === 0 && !loading && (
          <div className={styles.empty}>
            {query.trim() === '' ? 'This folder is empty' : `No matches for “${query}”`}
          </div>
        )}
      </div>
      {typed !== '' && (
        <div className={styles.typed} aria-live="polite">
          {typed}
        </div>
      )}
      {menu && (
        <ContextMenu x={menu.x} y={menu.y} items={menuItems()} onClose={() => setMenu(null)} />
      )}
    </div>
  );
}
