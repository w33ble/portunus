import { useVirtualizer } from '@tanstack/react-virtual';
import { type MouseEvent, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { Entry } from '../../api/types';
import { ENTRIES_MIME, encodeDragPayload } from '../../lib/dragData';
import { shownEntries } from '../../lib/listing';
import { clickSelect, pruneSelection, selectOnly } from '../../lib/selection';
import { IS_MAC } from '../../lib/platform';
import { visibleEntries } from '../../lib/sort';
import {
  copyToOtherPane,
  openEntry,
  requestDelete,
  requestNewFolder,
  requestRename,
  selectedEntries,
} from '../../stores/fileOps';
import type { PaneId } from '../../stores/paneTypes';
import { usePanes } from '../../stores/panes';
import { selectShowHidden, useSettings } from '../../stores/settings';
import { useUi } from '../../stores/ui';
import { ColumnHeader } from './ColumnHeader';
import { ContextMenu, type MenuItem } from './ContextMenu';
import { FileRow } from './FileRow';
import styles from './FileList.module.css';
import { useFileListKeys } from './useFileListKeys';
import { useListDrop } from './useListDrop';

const ROW_HEIGHT = 22;
const HEADER_HEIGHT = 22;

interface MenuState {
  x: number;
  y: number;
}

const MOD = IS_MAC ? '⌘' : 'Ctrl+';

export function FileList({ paneId }: { paneId: PaneId }) {
  const pane = usePanes((s) => s.panes[paneId]);
  const showHidden = useSettings(selectShowHidden);
  const scrollerRef = useRef<HTMLDivElement>(null);
  const [menu, setMenu] = useState<MenuState | null>(null);
  const drop = useListDrop(paneId, pane.path);

  const shown = shownEntries(pane.listing);
  const visible = useMemo(
    () => visibleEntries(shown, showHidden, pane.sort),
    [shown, showHidden, pane.sort]
  );
  const { selection } = pane;

  // Keyboard shortcuts work from launch: the active pane's list starts focused.
  useEffect(() => {
    if (useUi.getState().activePane === paneId) scrollerRef.current?.focus({ preventScroll: true });
  }, [paneId]);

  // Drop selections that scrolled out of existence (refresh, delete, hidden toggled).
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
  const onKeyDown = useFileListKeys({ paneId, visible, selection, pageRows, scrollerRef });

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
      setMenu({ x: event.clientX, y: event.clientY });
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
      event.dataTransfer.effectAllowed = 'copy';
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

  const loading = pane.status.kind === 'loading' || pane.status.kind === 'connecting';
  const items = virtualizer.getVirtualItems();

  return (
    <div
      className={styles.root}
      data-drop={drop.target?.kind === 'list' ? 'list' : 'none'}
      {...drop.listProps}
    >
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
          <div className={styles.empty}>This folder is empty</div>
        )}
      </div>
      {menu && (
        <ContextMenu x={menu.x} y={menu.y} items={menuItems()} onClose={() => setMenu(null)} />
      )}
    </div>
  );
}
