import { useShallow } from 'zustand/react/shallow';
import { summarize } from '../../lib/jobs';
import { useTransfers } from '../../stores/transfers';
import { type Tab, useUi } from '../../stores/ui';
import controls from '../ui/controls.module.css';
import { Icon } from '../ui/Icon';
import styles from './Titlebar.module.css';

const TABS: ReadonlyArray<{ id: Tab; label: string }> = [
  { id: 'browser', label: 'Browser' },
  { id: 'transfers', label: 'Transfers' },
];

export function Titlebar() {
  const tab = useUi((s) => s.tab);
  const setTab = useUi((s) => s.setTab);
  const openDialog = useUi((s) => s.openDialog);
  const active = useTransfers(useShallow((s) => summarize(s.jobs).active));

  return (
    <header className={styles.titlebar}>
      <div className={styles.tabs} role="tablist" aria-label="Views">
        {TABS.map(({ id, label }) => (
          <button
            key={id}
            type="button"
            role="tab"
            aria-selected={tab === id}
            className={styles.tab}
            data-active={tab === id}
            onClick={() => setTab(id)}
          >
            {label}
            {id === 'transfers' && active > 0 && (
              <span className={styles.badge} aria-label={`${active} active`}>
                {active}
              </span>
            )}
          </button>
        ))}
      </div>
      <button
        type="button"
        className={`${controls.iconButton} ${styles.settings}`}
        aria-label="Settings"
        title="Settings (⌘,)"
        onClick={() => openDialog({ kind: 'settings' })}
      >
        <Icon name="gear" />
      </button>
    </header>
  );
}
