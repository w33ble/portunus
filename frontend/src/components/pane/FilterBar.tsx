import type { PaneId } from '../../stores/paneTypes';
import { usePanes } from '../../stores/panes';
import controls from '../ui/controls.module.css';
import { Icon } from '../ui/Icon';
import styles from './FilterBar.module.css';

interface Props {
  paneId: PaneId;
  query: string;
  matches: number;
  total: number;
  /** Returns focus to the list; `pickBest` moves the cursor to the top match. */
  onLeave: (pickBest: boolean) => void;
}

/** ⌘F field that narrows the pane's list as you type. Esc closes it and restores the list. */
export function FilterBar({ paneId, query, matches, total, onLeave }: Props) {
  const close = () => {
    usePanes.getState().closeFilter(paneId);
    onLeave(false);
  };

  return (
    <div className={styles.bar}>
      <Icon name="search" className={styles.icon} />
      <input
        className={styles.input}
        data-filter-input=""
        aria-label="Filter"
        placeholder="Filter"
        value={query}
        autoFocus
        spellCheck={false}
        autoCapitalize="off"
        autoCorrect="off"
        onChange={(e) => usePanes.getState().setFilter(paneId, e.target.value)}
        onKeyDown={(e) => {
          if (e.key === 'Escape') {
            e.preventDefault();
            e.stopPropagation();
            close();
          } else if (e.key === 'Enter' || e.key === 'ArrowDown') {
            e.preventDefault();
            onLeave(true);
          }
        }}
      />
      {query.trim() !== '' && (
        <span className={styles.count}>
          {matches} of {total}
        </span>
      )}
      <button
        type="button"
        className={controls.iconButton}
        aria-label="Close filter"
        title="Close filter (Esc)"
        onClick={close}
      >
        <Icon name="close" />
      </button>
    </div>
  );
}
