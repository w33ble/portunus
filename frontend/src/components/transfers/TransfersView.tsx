import { useShallow } from 'zustand/react/shallow';
import { orderedJobs, summarize } from '../../lib/jobs';
import { useTransfers } from '../../stores/transfers';
import { TransferLimitsFields } from '../dialogs/TransferLimitsFields';
import controls from '../ui/controls.module.css';
import { JobRow } from './JobRow';
import styles from './TransfersView.module.css';

export function TransfersView() {
  const jobs = useTransfers(useShallow((s) => orderedJobs(s.jobs)));
  const finished = useTransfers((s) => summarize(s.jobs).finished);
  const clear = useTransfers((s) => s.clearFinished);

  return (
    <div className={styles.view}>
      <div className={styles.toolbar}>
        <TransferLimitsFields />
        <span className={styles.spacer} />
        <button
          type="button"
          className={controls.button}
          disabled={finished === 0}
          onClick={() => void clear()}
        >
          Clear finished
        </button>
      </div>
      {jobs.length === 0 ? (
        <div className={styles.empty}>
          <p>No transfers yet.</p>
          <p className={controls.hint}>Drag files between panes, or select files and press F5.</p>
        </div>
      ) : (
        <ul className={styles.list}>
          {jobs.map((job) => (
            <JobRow key={job.id} job={job} />
          ))}
        </ul>
      )}
    </div>
  );
}
