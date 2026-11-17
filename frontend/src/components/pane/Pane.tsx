import { type KeyboardEvent, type MouseEvent, useRef } from 'react';
import { historyButton, historyKey } from '../../lib/history';
import { IS_MAC, isEditable } from '../../lib/platform';
import { toggleBookmark } from '../../stores/bookmarks';
import { requestNewFolder } from '../../stores/fileOps';
import type { PaneId } from '../../stores/paneTypes';
import { usePanes } from '../../stores/panes';
import { useUi } from '../../stores/ui';
import { FileList } from './FileList';
import { Icon } from '../ui/Icon';
import { PaneBanner } from './PaneBanner';
import { PaneHeader } from './PaneHeader';
import styles from './Pane.module.css';
import type { PathBarHandle } from './PathBar';
import { useSwipeNavigation } from './useSwipeNavigation';

function go(paneId: PaneId, direction: 'back' | 'forward'): void {
  if (direction === 'back') usePanes.getState().goBack(paneId);
  else usePanes.getState().goForward(paneId);
}

/** Both halves of a back/forward click are prevented so the webview never navigates itself. */
function preventHistoryButton(event: MouseEvent): void {
  if (historyButton(event.button)) event.preventDefault();
}

/** `hidden` keeps the pane mounted (scroll, listing) while the other pane's preview stands in. */
export function Pane({ paneId, hidden = false }: { paneId: PaneId; hidden?: boolean }) {
  const active = useUi((s) => s.activePane === paneId);
  const setActivePane = useUi((s) => s.setActivePane);
  const pathBar = useRef<PathBarHandle>(null);
  const section = useRef<HTMLElement>(null);
  const swipeArrow = useRef<HTMLDivElement>(null);
  useSwipeNavigation(paneId, section, swipeArrow);

  const onKeyDown = (event: KeyboardEvent) => {
    const nav = historyKey(event, IS_MAC);
    // Alt+←/→ moves by word in text fields; leave editing alone.
    if (nav && !(event.altKey && isEditable(event.target))) {
      event.preventDefault();
      go(paneId, nav);
      return;
    }
    if (!(event.metaKey || event.ctrlKey)) return;
    const key = event.key.toLowerCase();
    if (key === 'l') {
      event.preventDefault();
      pathBar.current?.edit();
    } else if (key === 'r') {
      event.preventDefault();
      usePanes.getState().refresh(paneId);
    } else if (key === 'n' && event.shiftKey) {
      event.preventDefault();
      requestNewFolder(paneId);
    } else if (key === 'd') {
      event.preventDefault();
      void toggleBookmark(paneId);
    } else if (key === 'f') {
      event.preventDefault();
      const field = event.currentTarget.querySelector<HTMLInputElement>('[data-filter-input]');
      if (!field) usePanes.getState().openFilter(paneId);
      field?.focus();
      field?.select();
    }
  };

  // Mouse back/forward act on the pane under the pointer.
  const onMouseUp = (event: MouseEvent) => {
    const nav = historyButton(event.button);
    if (nav === null) return;
    event.preventDefault();
    go(paneId, nav);
  };

  return (
    <section
      ref={section}
      className={styles.pane}
      aria-label={paneId === 'left' ? 'Left pane' : 'Right pane'}
      data-pane={paneId}
      data-pane-active={active}
      hidden={hidden}
      data-file-drop-target
      onFocusCapture={() => setActivePane(paneId)}
      onMouseDownCapture={() => setActivePane(paneId)}
      onKeyDown={onKeyDown}
      onMouseDown={preventHistoryButton}
      onMouseUp={onMouseUp}
    >
      <PaneHeader paneId={paneId} pathBarRef={pathBar} />
      <div className={styles.list}>
        <FileList paneId={paneId} onEditPath={(prefill) => pathBar.current?.edit(prefill)} />
        <PaneBanner paneId={paneId} />
        <div ref={swipeArrow} className={styles.swipe} aria-hidden="true" data-swipe-arrow="">
          <Icon name="chevronLeft" />
        </div>
      </div>
    </section>
  );
}
