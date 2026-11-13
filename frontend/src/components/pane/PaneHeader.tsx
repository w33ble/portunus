import type { Ref } from 'react';
import { parentPath } from '../../lib/paths';
import { requestNewFolder } from '../../stores/fileOps';
import type { PaneId } from '../../stores/paneTypes';
import { usePanes } from '../../stores/panes';
import controls from '../ui/controls.module.css';
import { Icon } from '../ui/Icon';
import { ConnectionPicker } from './ConnectionPicker';
import styles from './PaneHeader.module.css';
import { PathBar, type PathBarHandle } from './PathBar';

interface Props {
  paneId: PaneId;
  pathBarRef: Ref<PathBarHandle>;
}

export function PaneHeader({ paneId, pathBarRef }: Props) {
  const pane = usePanes((s) => s.panes[paneId]);
  const noConnection = pane.connId === null;
  const atRoot = parentPath(pane.path) === pane.path;

  return (
    <div className={styles.header}>
      <ConnectionPicker paneId={paneId} />
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
