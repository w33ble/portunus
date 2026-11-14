import { type DragEvent, useCallback, useState } from 'react';
import type { Entry } from '../../api/types';
import { ENTRIES_MIME, decodeDragPayload, dragOperation } from '../../lib/dragData';
import { dropOnPane } from '../../stores/fileOps';
import type { PaneId } from '../../stores/paneTypes';

export type DropTarget = { kind: 'list' } | { kind: 'row'; path: string };

function carriesEntries(event: DragEvent): boolean {
  return Array.from(event.dataTransfer.types).includes(ENTRIES_MIME);
}

function accept(event: DragEvent): void {
  event.preventDefault();
  event.dataTransfer.dropEffect = dragOperation(event);
}

/**
 * In-app drag and drop. OS file drops are not handled here: Wails delivers them as drop:files
 * events keyed on the pane's data-file-drop-target element.
 */
export function useListDrop(paneId: PaneId, dir: string) {
  const [target, setTarget] = useState<DropTarget | null>(null);

  const finish = useCallback(
    (event: DragEvent, dstDir: string) => {
      event.preventDefault();
      event.stopPropagation();
      setTarget(null);
      const payload = decodeDragPayload(event.dataTransfer.getData(ENTRIES_MIME));
      if (payload) dropOnPane(paneId, payload, dstDir, dragOperation(event));
    },
    [paneId]
  );

  const listProps = {
    onDragOver(event: DragEvent) {
      if (!carriesEntries(event)) return;
      accept(event);
      setTarget((t) => (t?.kind === 'list' ? t : { kind: 'list' }));
    },
    onDragLeave(event: DragEvent) {
      if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setTarget(null);
    },
    onDrop(event: DragEvent) {
      if (carriesEntries(event)) finish(event, dir);
    },
  };

  const onRowDragOver = useCallback((entry: Entry, event: DragEvent) => {
    if (!carriesEntries(event)) return;
    accept(event);
    event.stopPropagation();
    setTarget((t) =>
      t?.kind === 'row' && t.path === entry.path ? t : { kind: 'row', path: entry.path }
    );
  }, []);

  const onRowDragLeave = useCallback((entry: Entry, event: DragEvent) => {
    if (event.currentTarget.contains(event.relatedTarget as Node | null)) return;
    setTarget((t) => (t?.kind === 'row' && t.path === entry.path ? { kind: 'list' } : t));
  }, []);

  const onRowDrop = useCallback(
    (entry: Entry, event: DragEvent) => {
      if (carriesEntries(event)) finish(event, entry.path);
    },
    [finish]
  );

  return {
    target,
    listProps,
    onRowDragOver,
    onRowDragLeave,
    onRowDrop,
    clear: () => setTarget(null),
  };
}
