import { useState } from 'react';
import { JobKind, State } from '../../api/enums';
import type { JobView } from '../../api/types';
import { formatBytes, formatEta, formatSpeed } from '../../lib/format';
import { isActiveState, progressFraction } from '../../lib/jobs';
import { useConnections } from '../../stores/connections';
import { useTransfers } from '../../stores/transfers';
import controls from '../ui/controls.module.css';
import { Icon } from '../ui/Icon';
import styles from './JobRow.module.css';
import { TaskList } from './TaskList';

const STATE_LABEL: Record<State, string> = {
  [State.$zero]: '',
  [State.StateQueued]: 'Queued',
  [State.StateRunning]: 'Running',
  [State.StateWaiting]: 'Waiting for your answer',
  [State.StateDone]: 'Done',
  [State.StateFailed]: 'Failed',
  [State.StateCanceled]: 'Canceled',
  [State.StateSkipped]: 'Skipped',
};

function useConnectionLabel(id: string): string {
  const label = useConnections((s) => s.connections.find((c) => c.id === id)?.label);
  return label ?? (id === 'local' ? 'Local' : id);
}

function Route({ job }: { job: JobView }) {
  const src = useConnectionLabel(job.srcConn);
  const dst = useConnectionLabel(job.dstConn);
  if (job.kind === JobKind.KindDelete) return <span>Deleting from {src}</span>;
  return (
    <span>
      {job.kind === JobKind.KindMove && 'Moving '}
      {src}
      <Icon name="arrowRight" size={12} className={styles.arrow} />
      {dst}:{job.dstDir}
    </span>
  );
}

export function JobRow({ job }: { job: JobView }) {
  const [open, setOpen] = useState(false);
  const cancel = useTransfers((s) => s.cancel);
  const retry = useTransfers((s) => s.retry);
  const resume = useTransfers((s) => s.resume);

  const state = job.state;
  const active = isActiveState(state);
  const activeTasks = job.active ?? [];
  const failedTasks = job.failed ?? [];
  const eta = formatEta(job.etaSeconds);
  const calculating = job.walking || (state === State.StateRunning && eta === null);
  const etaText = calculating ? 'calculating…' : eta === null ? null : `${eta} left`;
  const fraction = progressFraction(job);
  const hasDetails = activeTasks.length > 0 || failedTasks.length > 0 || Boolean(job.error);

  return (
    <li className={styles.job} data-state={state}>
      <div className={styles.top}>
        <button
          type="button"
          className={controls.iconButton}
          aria-label={open ? 'Hide details' : 'Show details'}
          aria-expanded={open}
          disabled={!hasDetails}
          onClick={() => setOpen((v) => !v)}
        >
          <Icon name={open ? 'chevronDown' : 'chevronRight'} />
        </button>
        <span className={styles.title} title={job.title}>
          {job.title}
        </span>
        <span className={styles.state} data-state={state}>
          {STATE_LABEL[state]}
        </span>
        {active && (
          <button type="button" className={controls.button} onClick={() => void cancel(job.id)}>
            Cancel
          </button>
        )}
        {state === State.StateCanceled && (
          <button type="button" className={controls.button} onClick={() => void resume(job.id)}>
            <Icon name="retry" size={12} />
            Resume
          </button>
        )}
        {state === State.StateFailed && job.failedFiles > 0 && (
          <button type="button" className={controls.button} onClick={() => void retry(job.id)}>
            <Icon name="retry" size={12} />
            Retry failed
          </button>
        )}
      </div>

      <div className={styles.route}>
        <Route job={job} />
      </div>

      <div
        className={styles.bar}
        role="progressbar"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={Math.round(fraction * 100)}
        data-state={state}
        data-walking={job.walking && job.totalBytes === 0}
      >
        <div className={styles.fill} style={{ width: `${fraction * 100}%` }} />
      </div>

      <div className={styles.stats}>
        <span>
          {job.kind !== JobKind.KindDelete && (
            <>
              {formatBytes(job.doneBytes)} of {formatBytes(job.totalBytes)}
              {job.walking && '+'} ·{' '}
            </>
          )}
          {job.doneFiles} of {job.totalFiles}
          {job.walking && '+'} {job.totalFiles === 1 ? 'item' : 'items'}
        </span>
        {job.failedFiles > 0 && <span className={styles.failed}>{job.failedFiles} failed</span>}
        {job.skippedFiles > 0 && <span>{job.skippedFiles} skipped</span>}
        <span className={styles.spacer} />
        {state === State.StateRunning && job.bytesPerSec > 0 && (
          <span>{formatSpeed(job.bytesPerSec)}</span>
        )}
        {active && etaText !== null && <span>{etaText}</span>}
      </div>

      {open && (
        <div className={styles.details}>
          {job.error && <p className={styles.jobError}>{job.error}</p>}
          <TaskList heading="In progress" tasks={activeTasks} />
          <TaskList
            heading={
              job.failedFiles > failedTasks.length
                ? `Failed (showing ${failedTasks.length} of ${job.failedFiles})`
                : 'Failed'
            }
            tasks={failedTasks}
          />
        </div>
      )}
    </li>
  );
}
