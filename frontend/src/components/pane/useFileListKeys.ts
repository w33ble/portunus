import type { KeyboardEvent, RefObject } from 'react';
import type { Entry } from '../../api/types';
import { type Selection, jumpTo, moveCursor, selectAll } from '../../lib/selection';
import { copyToOtherPane, openEntry, requestDelete, requestRename } from '../../stores/fileOps';
import { type PaneId, otherPane } from '../../stores/paneTypes';
import { usePanes } from '../../stores/panes';
import { useUi } from '../../stores/ui';

interface Args {
  paneId: PaneId;
  visible: readonly Entry[];
  selection: Selection;
  pageRows: number;
  scrollerRef: RefObject<HTMLDivElement | null>;
}

function focusPane(id: PaneId): void {
  useUi.getState().setActivePane(id);
  document
    .querySelector<HTMLElement>(`[data-pane="${id}"] [data-filelist]`)
    ?.focus({ preventScroll: true });
}

/** Keyboard handling for a focused file list. Pane-wide shortcuts (⌘L, ⌘R) live in Pane. */
export function useFileListKeys({ paneId, visible, selection, pageRows }: Args) {
  const panes = usePanes.getState();
  const paths = visible.map((e) => e.path);
  const cursorEntry = visible.find((e) => e.path === selection.cursor);

  return (event: KeyboardEvent) => {
    const mod = event.metaKey || event.ctrlKey;
    const move = (delta: number) => {
      event.preventDefault();
      panes.setSelection(paneId, moveCursor(selection, paths, delta, event.shiftKey));
    };

    switch (event.key) {
      case 'ArrowDown':
        if (mod && !event.shiftKey) {
          event.preventDefault();
          if (cursorEntry) openEntry(paneId, cursorEntry);
        } else move(1);
        break;
      case 'ArrowUp':
        if (mod && !event.shiftKey) {
          event.preventDefault();
          panes.goUp(paneId);
        } else move(-1);
        break;
      case 'PageDown':
        move(pageRows);
        break;
      case 'PageUp':
        move(-pageRows);
        break;
      case 'Home':
        event.preventDefault();
        panes.setSelection(paneId, jumpTo(selection, paths, 'first', event.shiftKey));
        break;
      case 'End':
        event.preventDefault();
        panes.setSelection(paneId, jumpTo(selection, paths, 'last', event.shiftKey));
        break;
      case 'Enter':
        event.preventDefault();
        if (cursorEntry) openEntry(paneId, cursorEntry);
        break;
      case 'Backspace':
        event.preventDefault();
        if (mod) requestDelete(paneId);
        else panes.goUp(paneId);
        break;
      case 'Delete':
        event.preventDefault();
        requestDelete(paneId);
        break;
      case 'F2':
        event.preventDefault();
        requestRename(paneId);
        break;
      case 'F5':
        event.preventDefault();
        copyToOtherPane(paneId);
        break;
      case 'Escape':
        panes.setSelection(paneId, { selected: new Set(), anchor: null, cursor: selection.cursor });
        break;
      case 'Tab':
        event.preventDefault();
        focusPane(otherPane(paneId));
        break;
      case 'a':
      case 'A':
        if (mod) {
          event.preventDefault();
          panes.setSelection(paneId, selectAll(paths, selection));
        }
        break;
      default:
        break;
    }
  };
}
