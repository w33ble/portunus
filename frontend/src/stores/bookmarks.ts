import { AppError, toAppError } from '../api/errors';
import type { Bookmark } from '../api/types';
import { bookmarkAt } from '../lib/catalog';
import { catalogNow, useConnections } from './connections';
import type { PaneId } from './paneTypes';
import { usePanes } from './panes';
import { useUi } from './ui';

/** Opens a bookmark in a pane, connecting first when the pane is on another connection. */
export async function openBookmark(paneId: PaneId, bookmark: Bookmark): Promise<void> {
  const panes = usePanes.getState();
  if (panes.panes[paneId].connId === bookmark.connId) {
    panes.navigate(paneId, bookmark.path);
    return;
  }
  const conn = catalogNow().find((c) => c.id === bookmark.connId);
  if (conn === undefined) {
    useUi
      .getState()
      .showError(
        'Couldn’t open bookmark',
        new AppError('not_exist', 'The connection this bookmark belongs to no longer exists.')
      );
    return;
  }
  await panes.connectTo(paneId, conn.target, conn.name, bookmark.path);
}

/** Whether a connection can hold bookmarks: Local, an alias, a saved host or an S3 profile. */
export function canBookmark(connId: string | null): boolean {
  return connId !== null && catalogNow().some((c) => c.id === connId);
}

/** Bookmarks each folder on the pane's connection that isn't bookmarked yet. */
export async function bookmarkFolders(paneId: PaneId, paths: readonly string[]): Promise<void> {
  const { connId } = usePanes.getState().panes[paneId];
  if (connId === null) return;
  for (const path of paths) {
    const { bookmarks, addBookmark } = useConnections.getState();
    if (bookmarkAt(bookmarks, connId, path)) continue;
    try {
      await addBookmark(connId, path);
    } catch (e) {
      useUi.getState().showError('Couldn’t add bookmark', toAppError(e));
      return;
    }
  }
}

/** Bookmarks a folder (default: the pane's own), or asks to remove its bookmark when it has one. */
export async function toggleBookmark(
  paneId: PaneId,
  path = usePanes.getState().panes[paneId].path
): Promise<void> {
  const { connId } = usePanes.getState().panes[paneId];
  if (connId === null) return;
  const existing = bookmarkAt(useConnections.getState().bookmarks, connId, path);
  if (existing) {
    useUi.getState().openDialog({ kind: 'removeBookmark', bookmark: existing });
    return;
  }
  await bookmarkFolders(paneId, [path]);
}
