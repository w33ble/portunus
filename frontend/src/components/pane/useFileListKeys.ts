import type { KeyboardEvent, RefObject } from 'react';
import type { Entry } from '../../api/types';
import { EMPTY_TYPE_AHEAD, type TypeAhead, typeAheadIndex, typeKey } from '../../lib/match';
import { type Selection, jumpTo, moveCursor, selectAll, selectOnly } from '../../lib/selection';
import { copyToOtherPane, openEntry, requestDelete, requestRename } from '../../stores/fileOps';
import { type PaneId, otherPane } from '../../stores/paneTypes';
import { usePanes } from '../../stores/panes';
import { useUi } from '../../stores/ui';

interface Args {
  paneId: PaneId;
  visible: readonly Entry[];
  selection: Selection;
  pageRows: number;
  filtering: boolean;
  typeAhead: RefObject<TypeAhead>;
  /** Shows the typed prefix; empty once it resets. */
  onTyped: (text: string) => void;
  /** Opens the path editor with `prefill` typed in. */
  onEditPath: (prefill: string) => void;
}

function focusPane(id: PaneId): void {
  useUi.getState().setActivePane(id);
  document
    .querySelector<HTMLElement>(`[data-pane="${id}"] [data-filelist]`)
    ?.focus({ preventScroll: true });
}

function focusPreview(): void {
  document.querySelector<HTMLElement>('[data-preview-body]')?.focus({ preventScroll: true });
}

/** Keyboard handling for a focused file list. Pane-wide shortcuts (⌘L, ⌘R, ⌘F) live in Pane. */
export function useFileListKeys(args: Args) {
  const { paneId, visible, selection, pageRows, filtering, typeAhead } = args;
  const panes = usePanes.getState();
  const paths = visible.map((e) => e.path);
  const cursorEntry = visible.find((e) => e.path === selection.cursor);

  return (event: KeyboardEvent) => {
    const previewing = useUi.getState().preview === paneId;
    const mod = event.metaKey || event.ctrlKey;
    const move = (delta: number) => {
      event.preventDefault();
      panes.setSelection(paneId, moveCursor(selection, paths, delta, event.shiftKey));
    };

    // Finder-style: printable keys jump to the first name starting with what was typed.
    const typeToFind = () => {
      if (mod || event.nativeEvent.isComposing || [...event.key].length !== 1) return;
      const next = typeKey(typeAhead.current, event.key, Date.now());
      // A lone space toggles the preview; after typed letters it's part of the name.
      if (next.text === ' ') {
        event.preventDefault();
        useUi.getState().togglePreview(paneId);
        return;
      }
      event.preventDefault();
      if (next.text === '/' || next.text === '~') {
        typeAhead.current = EMPTY_TYPE_AHEAD;
        args.onEditPath(next.text);
        return;
      }
      typeAhead.current = next;
      args.onTyped(next.text);
      const cursor = selection.cursor === null ? -1 : paths.indexOf(selection.cursor);
      const target =
        visible[
          typeAheadIndex(
            visible.map((e) => e.name),
            next.text,
            cursor
          )
        ];
      if (target) panes.setSelection(paneId, selectOnly(target.path));
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
        if (previewing) useUi.getState().closePreview();
        else if (filtering) panes.closeFilter(paneId);
        else
          panes.setSelection(paneId, {
            selected: new Set(),
            anchor: null,
            cursor: selection.cursor,
          });
        break;
      case 'Tab':
        event.preventDefault();
        // The preview stands in for the other pane, so Tab goes to it instead.
        if (previewing) focusPreview();
        else focusPane(otherPane(paneId));
        break;
      case 'a':
      case 'A':
        if (mod) {
          event.preventDefault();
          panes.setSelection(paneId, selectAll(paths, selection));
        } else typeToFind();
        break;
      default:
        typeToFind();
        break;
    }
  };
}
