import type { Entry } from '../api/types';

export type SortKey = 'name' | 'size' | 'modTime';
export type SortDir = 'asc' | 'desc';

export interface SortState {
  key: SortKey;
  dir: SortDir;
}

export const DEFAULT_SORT: SortState = { key: 'name', dir: 'asc' };

const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' });

/** Case-insensitive natural order (web2 before web10) by a label. */
export function sortByLabel<T>(items: readonly T[], label: (item: T) => string): T[] {
  return items.toSorted((a, b) => collator.compare(label(a), label(b)));
}

function compareBy(key: SortKey, a: Entry, b: Entry): number {
  switch (key) {
    case 'name':
      return collator.compare(a.name, b.name);
    case 'size':
      return a.size - b.size;
    case 'modTime':
      return Date.parse(a.modTime) - Date.parse(b.modTime);
    default: {
      const unreachable: never = key;
      return unreachable;
    }
  }
}

/** Directories always sort before files; ties fall back to name so order is stable. */
export function sortEntries(entries: readonly Entry[], sort: SortState): Entry[] {
  const sign = sort.dir === 'asc' ? 1 : -1;
  return entries.toSorted((a, b) => {
    if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
    const primary = compareBy(sort.key, a, b);
    if (primary !== 0 && !Number.isNaN(primary)) return primary * sign;
    return collator.compare(a.name, b.name) || (a.name < b.name ? -1 : a.name > b.name ? 1 : 0);
  });
}

/** Clicking the active column flips direction; a new column starts ascending (dates start newest first). */
export function toggleSort(current: SortState, key: SortKey): SortState {
  if (current.key === key) return { key, dir: current.dir === 'asc' ? 'desc' : 'asc' };
  return { key, dir: key === 'modTime' ? 'desc' : 'asc' };
}

export function isHidden(entry: Entry): boolean {
  return entry.name.startsWith('.');
}

export function visibleEntries(
  entries: readonly Entry[],
  showHidden: boolean,
  sort: SortState
): Entry[] {
  return sortEntries(showHidden ? entries : entries.filter((e) => !isHidden(e)), sort);
}
