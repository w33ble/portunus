import type { Entry } from '../api/types';

/**
 * A streamed directory listing. FileService.List emits `files:page` events and resolves with
 * the total count; the two can arrive in either order, so the listing is complete only when
 * the received entry count reaches the total.
 */
export interface Listing {
  requestId: string;
  entries: readonly Entry[];
  /** Null until the List call resolves. */
  total: number | null;
  /** On refresh, the previous entries stay visible until the new listing completes. */
  replacing: readonly Entry[] | null;
}

export const EMPTY_LISTING: Listing = { requestId: '', entries: [], total: null, replacing: null };

export function startListing(requestId: string, replacing: readonly Entry[] | null): Listing {
  return { requestId, entries: [], total: null, replacing };
}

export function isComplete(l: Listing): boolean {
  return l.total !== null && l.entries.length >= l.total;
}

function settle(l: Listing): Listing {
  return isComplete(l) && l.replacing !== null ? { ...l, replacing: null } : l;
}

/** Pages for other requests (a superseded listing) are ignored. */
export function applyPage(l: Listing, requestId: string, entries: readonly Entry[]): Listing {
  if (requestId !== l.requestId || entries.length === 0) return l;
  return settle({ ...l, entries: l.entries.concat(entries) });
}

export function applyTotal(l: Listing, requestId: string, total: number): Listing {
  if (requestId !== l.requestId) return l;
  return settle({ ...l, total });
}

/** What the pane renders: the old entries while a refresh is still streaming in. */
export function shownEntries(l: Listing): readonly Entry[] {
  return l.replacing ?? l.entries;
}

/** Coalesces rapid pages so a huge listing re-sorts a few times a second, not once per page. */
export interface PageBatcher {
  push(requestId: string, entries: readonly Entry[]): void;
  flush(): void;
}

export function createPageBatcher(
  onFlush: (pages: ReadonlyMap<string, readonly Entry[]>) => void,
  delayMs: number
): PageBatcher {
  let pending = new Map<string, Entry[]>();
  let timer: ReturnType<typeof setTimeout> | null = null;

  const flush = () => {
    if (timer !== null) clearTimeout(timer);
    timer = null;
    if (pending.size === 0) return;
    const batch = pending;
    pending = new Map();
    onFlush(batch);
  };

  return {
    push(requestId, entries) {
      const bucket = pending.get(requestId);
      if (bucket) bucket.push(...entries);
      else pending.set(requestId, [...entries]);
      timer ??= setTimeout(flush, delayMs);
    },
    flush,
  };
}
