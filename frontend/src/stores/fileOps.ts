import * as api from '../api';
import { toAppError } from '../api/errors';
import type { Entry } from '../api/types';
import { type DragPayload, isSameDirectoryDrop } from '../lib/dragData';
import { selectOnly } from '../lib/selection';
import { visibleEntries } from '../lib/sort';
import { type PaneId, otherPane } from './paneTypes';
import { usePanes } from './panes';
import { useSettings, selectShowHidden } from './settings';
import { useUi } from './ui';

export const LOCAL_CONN = 'local';

/** Selected entries that are still listed and visible, in display order. */
export function selectedEntries(id: PaneId): Entry[] {
  const pane = usePanes.getState().panes[id];
  const showHidden = selectShowHidden(useSettings.getState());
  const visible = visibleEntries(
    pane.listing.replacing ?? pane.listing.entries,
    showHidden,
    pane.sort
  );
  return visible.filter((e) => pane.selection.selected.has(e.path));
}

async function startCopy(req: {
  srcConn: string;
  sources: string[];
  dstConn: string;
  dstDir: string;
}): Promise<void> {
  try {
    await api.transfers.copy(req);
  } catch (e) {
    useUi.getState().showError('Couldn’t start the copy', toAppError(e));
  }
}

export function copyToOtherPane(id: PaneId): void {
  const source = usePanes.getState().panes[id];
  const dest = usePanes.getState().panes[otherPane(id)];
  const entries = selectedEntries(id);
  if (source.connId === null || dest.connId === null || entries.length === 0) return;
  void startCopy({
    srcConn: source.connId,
    sources: entries.map((e) => e.path),
    dstConn: dest.connId,
    dstDir: dest.path,
  });
}

/** In-app drop: `dstDir` is the pane's directory or a directory row inside it. */
export function dropOnPane(id: PaneId, payload: DragPayload, dstDir: string): void {
  const dest = usePanes.getState().panes[id];
  if (dest.connId === null) return;
  if (isSameDirectoryDrop(payload, { connId: dest.connId, dir: dstDir })) return;
  void startCopy({
    srcConn: payload.connId,
    sources: payload.paths,
    dstConn: dest.connId,
    dstDir,
  });
}

/** OS drop: dropped paths are always local. */
export function dropFromOs(id: PaneId, paths: readonly string[]): void {
  const dest = usePanes.getState().panes[id];
  if (dest.connId === null || paths.length === 0) return;
  void startCopy({
    srcConn: LOCAL_CONN,
    sources: [...paths],
    dstConn: dest.connId,
    dstDir: dest.path,
  });
}

export function requestDelete(id: PaneId): void {
  const pane = usePanes.getState().panes[id];
  const entries = selectedEntries(id);
  if (pane.connId === null || entries.length === 0) return;
  useUi.getState().openDialog({
    kind: 'confirmDelete',
    paneId: id,
    connId: pane.connId,
    targets: entries.map((e) => ({ path: e.path, name: e.name, isDir: e.isDir })),
  });
}

export async function confirmDelete(connId: string, paths: string[]): Promise<void> {
  try {
    await api.transfers.remove(connId, paths);
  } catch (e) {
    useUi.getState().showError('Couldn’t delete', toAppError(e));
  }
}

export function requestRename(id: PaneId): void {
  const [only] = selectedEntries(id);
  if (only === undefined) return;
  useUi.getState().openDialog({ kind: 'rename', paneId: id, path: only.path, name: only.name });
}

export function requestNewFolder(id: PaneId): void {
  if (usePanes.getState().panes[id].connId === null) return;
  useUi.getState().openDialog({ kind: 'newFolder', paneId: id });
}

/** Throws AppError so the dialog can show it inline. */
export async function createFolder(id: PaneId, name: string): Promise<void> {
  const pane = usePanes.getState().panes[id];
  if (pane.connId === null) return;
  const created = await api.files.mkdir(pane.connId, pane.path, name);
  usePanes.getState().refresh(id);
  usePanes.getState().setSelection(id, selectOnly(created.path));
}

export async function renameEntry(id: PaneId, path: string, name: string): Promise<void> {
  const pane = usePanes.getState().panes[id];
  if (pane.connId === null) return;
  const renamed = await api.files.rename(pane.connId, path, name);
  usePanes.getState().refresh(id);
  usePanes.getState().setSelection(id, selectOnly(renamed.path));
}

export function openEntry(id: PaneId, entry: Entry): void {
  if (entry.isDir) usePanes.getState().navigate(id, entry.path);
}
