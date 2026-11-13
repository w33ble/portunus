import { type DragEvent, type MouseEvent, memo } from 'react';
import type { Entry } from '../../api/types';
import { formatBytes, formatModified } from '../../lib/format';
import type { PaneId } from '../../stores/paneTypes';
import { Icon } from '../ui/Icon';
import styles from './FileList.module.css';

interface Props {
  paneId: PaneId;
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
  paneId,
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
  // Only a folder's icon and name accept drops; the rest of the row falls through to the list,
  // which drops into the pane's current directory. The data-file-drop-target attributes let
  // Wails report Finder drops onto the name with the folder's path.
  const folderTarget = entry.isDir
    ? {
        'data-drop': dropTarget,
        'data-file-drop-target': '',
        'data-pane': paneId,
        'data-path': entry.path,
        onDragOver: (e: DragEvent) => onDragOver(entry, e),
        onDragLeave: (e: DragEvent) => onDragLeave(entry, e),
        onDrop: (e: DragEvent) => onDrop(entry, e),
      }
    : {};

  return (
    <div
      role="option"
      aria-selected={selected}
      aria-posinset={index + 1}
      className={styles.row}
      data-selected={selected}
      data-cursor={cursor}
      data-dir={entry.isDir}
      style={{ transform: `translateY(${top}px)` }}
      draggable
      onClick={(e) => onClick(entry, e)}
      onDoubleClick={() => onOpen(entry)}
      onContextMenu={(e) => onContextMenu(entry, e)}
      onDragStart={(e) => onDragStart(entry, e)}
    >
      <span
        className={styles.name}
        title={entry.link ? `${entry.name} → ${entry.link}` : entry.name}
      >
        <span className={styles.target} {...folderTarget}>
          <Icon
            name={iconFor(entry)}
            className={styles.icon}
            data-kind={entry.isDir ? 'dir' : 'file'}
          />
          <span className={styles.nameText}>{entry.name}</span>
        </span>
      </span>
      <span className={styles.size}>{entry.isDir ? '—' : formatBytes(entry.size)}</span>
      <span className={styles.modified}>{formatModified(entry.modTime)}</span>
    </div>
  );
});
