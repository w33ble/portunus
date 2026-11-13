import { type DragEvent, type MouseEvent, memo } from 'react';
import type { Entry } from '../../api/types';
import { formatBytes, formatModified } from '../../lib/format';
import { Icon } from '../ui/Icon';
import styles from './FileList.module.css';

interface Props {
  entry: Entry;
  index: number;
  top: number;
  selected: boolean;
  cursor: boolean;
  dropTarget: boolean;
  onClick: (entry: Entry, event: MouseEvent) => void;
  onOpen: (entry: Entry) => void;
  onContextMenu: (entry: Entry, event: MouseEvent) => void;
  onDragStart: (entry: Entry, event: DragEvent) => void;
  onDragOver: (entry: Entry, event: DragEvent) => void;
  onDragLeave: (entry: Entry, event: DragEvent) => void;
  onDrop: (entry: Entry, event: DragEvent) => void;
}

function iconFor(entry: Entry) {
  if (entry.link) return 'link';
  return entry.isDir ? 'folder' : 'file';
}

export const FileRow = memo(function FileRow({
  entry,
  index,
  top,
  selected,
  cursor,
  dropTarget,
  onClick,
  onOpen,
  onContextMenu,
  onDragStart,
  onDragOver,
  onDragLeave,
  onDrop,
}: Props) {
  return (
    <div
      role="option"
      aria-selected={selected}
      aria-posinset={index + 1}
      className={styles.row}
      data-selected={selected}
      data-cursor={cursor}
      data-drop={dropTarget}
      data-dir={entry.isDir}
      style={{ transform: `translateY(${top}px)` }}
      draggable
      onClick={(e) => onClick(entry, e)}
      onDoubleClick={() => onOpen(entry)}
      onContextMenu={(e) => onContextMenu(entry, e)}
      onDragStart={(e) => onDragStart(entry, e)}
      onDragOver={entry.isDir ? (e) => onDragOver(entry, e) : undefined}
      onDragLeave={entry.isDir ? (e) => onDragLeave(entry, e) : undefined}
      onDrop={entry.isDir ? (e) => onDrop(entry, e) : undefined}
    >
      <span
        className={styles.name}
        title={entry.link ? `${entry.name} → ${entry.link}` : entry.name}
      >
        <Icon
          name={iconFor(entry)}
          className={styles.icon}
          data-kind={entry.isDir ? 'dir' : 'file'}
        />
        <span className={styles.nameText}>{entry.name}</span>
      </span>
      <span className={styles.size}>{entry.isDir ? '—' : formatBytes(entry.size)}</span>
      <span className={styles.modified}>{formatModified(entry.modTime)}</span>
    </div>
  );
});
