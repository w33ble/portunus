import type { SortKey, SortState } from '../../lib/sort';
import { Icon } from '../ui/Icon';
import styles from './FileList.module.css';

const COLUMNS: ReadonlyArray<{ key: SortKey; label: string; className: string }> = [
  { key: 'name', label: 'Name', className: styles.headName ?? '' },
  { key: 'size', label: 'Size', className: styles.headSize ?? '' },
  { key: 'modTime', label: 'Modified', className: styles.headModified ?? '' },
];

interface Props {
  sort: SortState;
  onSort: (key: SortKey) => void;
}

export function ColumnHeader({ sort, onSort }: Props) {
  return (
    <div className={styles.header} role="row">
      {COLUMNS.map(({ key, label, className }) => (
        <button
          key={key}
          type="button"
          role="columnheader"
          className={`${styles.headCell} ${className}`}
          aria-sort={sort.key === key ? (sort.dir === 'asc' ? 'ascending' : 'descending') : 'none'}
          onClick={() => onSort(key)}
          tabIndex={-1}
        >
          {label}
          {sort.key === key && <Icon name={sort.dir === 'asc' ? 'sortUp' : 'sortDown'} size={12} />}
        </button>
      ))}
    </div>
  );
}
