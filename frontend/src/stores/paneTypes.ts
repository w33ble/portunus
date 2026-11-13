export type PaneId = 'left' | 'right';

export const PANE_IDS: readonly PaneId[] = ['left', 'right'];

export function otherPane(id: PaneId): PaneId {
  return id === 'left' ? 'right' : 'left';
}

export function parsePaneId(value: string): PaneId | null {
  return value === 'left' || value === 'right' ? value : null;
}
