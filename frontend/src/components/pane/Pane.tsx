import { type KeyboardEvent, useRef } from 'react';
import { historyKey } from '../../lib/history';
import { IS_MAC, isEditable } from '../../lib/platform';
import { requestNewFolder } from '../../stores/fileOps';
import type { PaneId } from '../../stores/paneTypes';
import { usePanes } from '../../stores/panes';
import { useUi } from '../../stores/ui';
import { FileList } from './FileList';
import { PaneBanner } from './PaneBanner';
import { PaneHeader } from './PaneHeader';
import styles from './Pane.module.css';
import type { PathBarHandle } from './PathBar';

export function Pane({ paneId }: { paneId: PaneId }) {
  const active = useUi((s) => s.activePane === paneId);
  const setActivePane = useUi((s) => s.setActivePane);
  const pathBar = useRef<PathBarHandle>(null);

  const onKeyDown = (event: KeyboardEvent) => {
    const nav = historyKey(event, IS_MAC);
    // Alt+←/→ moves by word in text fields; leave editing alone.
    if (nav && !(event.altKey && isEditable(event.target))) {
      event.preventDefault();
      if (nav === 'back') usePanes.getState().goBack(paneId);
      else usePanes.getState().goForward(paneId);
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
    }
  };

  return (
    <section
      className={styles.pane}
      aria-label={paneId === 'left' ? 'Left pane' : 'Right pane'}
      data-pane={paneId}
      data-pane-active={active}
      data-file-drop-target
      onFocusCapture={() => setActivePane(paneId)}
      onMouseDownCapture={() => setActivePane(paneId)}
      onKeyDown={onKeyDown}
    >
      <PaneHeader paneId={paneId} pathBarRef={pathBar} />
      <div className={styles.list}>
        <FileList paneId={paneId} />
        <PaneBanner paneId={paneId} />
      </div>
    </section>
  );
}
