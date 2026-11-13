import { useShallow } from 'zustand/react/shallow';
import { formatSpeed } from '../../lib/format';
import { summarize } from '../../lib/jobs';
import { useTransfers } from '../../stores/transfers';
import { useUi } from '../../stores/ui';
import styles from './StatusBar.module.css';

export function StatusBar() {
  const { active, bytesPerSec } = useTransfers(useShallow((s) => summarize(s.jobs)));
  const setTab = useUi((s) => s.setTab);

  return (
    <footer className={styles.bar}>
      <button type="button" className={styles.summary} onClick={() => setTab('transfers')}>
        {active === 0
          ? 'No active transfers'
          : `${active} active ${active === 1 ? 'transfer' : 'transfers'}`}
      </button>
      <span className={styles.speed}>{active > 0 ? formatSpeed(bytesPerSec) : ''}</span>
    </footer>
  );
}
