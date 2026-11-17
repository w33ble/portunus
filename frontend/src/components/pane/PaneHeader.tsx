import type { Ref } from 'react';
import { historyShortcut } from '../../lib/history';
import { baseName, parentPath } from '../../lib/paths';
import { IS_MAC } from '../../lib/platform';
import { useConnections } from '../../stores/connections';
import { requestNewFolder } from '../../stores/fileOps';
import type { PaneId } from '../../stores/paneTypes';
import { usePanes } from '../../stores/panes';
import controls from '../ui/controls.module.css';
import { Icon } from '../ui/Icon';
import { BookmarksMenu } from './BookmarksMenu';
import { ConnectionPicker } from './ConnectionPicker';
import styles from './PaneHeader.module.css';
import { PathBar, type PathBarHandle } from './PathBar';

interface Props {
  paneId: PaneId;
  pathBarRef: Ref<PathBarHandle>;
}

export function PaneHeader({ paneId, pathBarRef }: Props) {
  const pane = usePanes((s) => s.panes[paneId]);
  const color = useConnections((s) =>
    pane.connId === null ? undefined : s.prefs[pane.connId]?.color
  );
  const noConnection = pane.connId === null;
  const atRoot = parentPath(pane.path) === pane.path;
  const backTo = pane.history.back.at(-1)?.path;
  const forwardTo = pane.history.forward.at(-1)?.path;

  return (
    <div className={styles.header}>
      <ConnectionPicker paneId={paneId} />
      {color && (
        <span
          className={styles.tag}
          style={{ backgroundColor: `var(--tag-${color})` }}
          title={`${color} tag`}
          aria-hidden="true"
        />
      )}
      <div className={styles.nav}>
        <button
          type="button"
          className={controls.iconButton}
          aria-label="Back"
          title={navTitle('back', backTo)}
          disabled={noConnection || backTo === undefined}
          onClick={() => usePanes.getState().goBack(paneId)}
        >
          <Icon name="chevronLeft" />
        </button>
        <button
          type="button"
          className={controls.iconButton}
          aria-label="Forward"
          title={navTitle('forward', forwardTo)}
          disabled={noConnection || forwardTo === undefined}
          onClick={() => usePanes.getState().goForward(paneId)}
        >
          <Icon name="chevronRight" />
        </button>
      </div>
      <button
        type="button"
        className={controls.iconButton}
        aria-label="Up one level"
        title="Up one level (⌘↑)"
        disabled={noConnection || atRoot}
        onClick={() => usePanes.getState().goUp(paneId)}
      >
        <Icon name="up" />
      </button>
      <PathBar
        ref={pathBarRef}
        path={pane.path}
        disabled={noConnection}
        onNavigate={(path) => usePanes.getState().navigate(paneId, path)}
      />
      <BookmarksMenu paneId={paneId} />
      <button
        type="button"
        className={controls.iconButton}
        aria-label="Refresh"
        title="Refresh (⌘R)"
        disabled={noConnection}
        onClick={() => usePanes.getState().refresh(paneId)}
      >
        <Icon name="refresh" />
      </button>
      <button
        type="button"
        className={controls.iconButton}
        aria-label="New folder"
        title="New folder (⌘⇧N)"
        disabled={noConnection}
        onClick={() => requestNewFolder(paneId)}
      >
        <Icon name="newFolder" />
      </button>
    </div>
  );
}

/** "Back to docs (⌘[)": names the destination folder and the shortcut. */
function navTitle(direction: 'back' | 'forward', to: string | undefined): string {
  const label = direction === 'back' ? 'Back' : 'Forward';
  const dest = to === undefined ? '' : ` to ${baseName(to) || to}`;
  return `${label}${dest} (${historyShortcut(direction, IS_MAC)})`;
}
