import type { Entry } from '../api/types';

/** A directory listing that fills as pages are pulled from the backend. */
export interface Listing {
  requestId: string;
  entries: readonly Entry[];
  /** True once the backend delivered the last page (or the listing failed). */
  done: boolean;
  /** On refresh, the previous entries stay visible until the new listing completes. */
  replacing: readonly Entry[] | null;
}

export const EMPTY_LISTING: Listing = { requestId: '', entries: [], done: false, replacing: null };

export function startListing(requestId: string, replacing: readonly Entry[] | null): Listing {
  return { requestId, entries: [], done: false, replacing };
}

/** Pages for other requests (a superseded listing) are ignored. */
export function applyPage(l: Listing, requestId: string, entries: readonly Entry[]): Listing {
  if (requestId !== l.requestId || entries.length === 0) return l;
  return { ...l, entries: l.entries.concat(entries) };
}

/** The last page arrived: a refresh's new entries replace the old ones. */
export function finishListing(l: Listing, requestId: string): Listing {
  if (requestId !== l.requestId) return l;
  return { ...l, done: true, replacing: null };
}

/** A failed listing keeps what it got, except a refresh, which keeps showing the old entries. */
export function failListing(l: Listing, requestId: string): Listing {
  if (requestId !== l.requestId || l.replacing !== null) return l;
  return { ...l, done: true };
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
