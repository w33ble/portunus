import { describeError } from '../../api/errors';
import type { PaneId } from '../../stores/paneTypes';
import { usePanes } from '../../stores/panes';
import controls from '../ui/controls.module.css';
import { Icon } from '../ui/Icon';
import styles from './PaneBanner.module.css';

/** Floats over the list so state changes never move the rows. */
export function PaneBanner({ paneId }: { paneId: PaneId }) {
  const status = usePanes((s) => s.panes[paneId].status);

  if (status.kind === 'connecting') {
    return (
      <div className={styles.banner} role="status">
        <span className={styles.spinner} aria-hidden="true" />
        Connecting to {status.label}…
      </div>
    );
  }

  if (status.kind === 'error') {
    return (
      <div className={`${styles.banner} ${styles.error}`} role="alert">
        <Icon name="warning" />
        <span className={styles.message}>{describeError(status.error)}</span>
        <button
          type="button"
          className={controls.button}
          onClick={() => usePanes.getState().refresh(paneId)}
        >
          Retry
        </button>
        <button
          type="button"
          className={controls.button}
          onClick={() => usePanes.getState().goUp(paneId)}
        >
          Up
        </button>
        <button
          type="button"
          className={controls.iconButton}
          aria-label="Dismiss"
          onClick={() => usePanes.getState().dismissError(paneId)}
        >
          <Icon name="close" />
        </button>
      </div>
    );
  }

  if (status.kind === 'loading') {
    return <span className={styles.loading} role="status" aria-label="Loading" />;
  }

  return null;
}
