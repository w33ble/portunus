import { AppError, toAppError } from './errors';
import type { Entry, ListPage } from './types';

/** A bound call whose in-flight request can be aborted (Wails' CancellablePromise). */
export interface Cancellable<T> extends PromiseLike<T> {
  cancel(): unknown;
}

/** The three FileService calls a pull-based listing is built from. */
export interface ListBackend {
  start(connId: string, dir: string): Cancellable<ListPage>;
  next(cursor: string): Cancellable<ListPage>;
  close(cursor: string): PromiseLike<unknown>;
}

export interface Listing {
  /** Resolves once every page was delivered; rejects with an AppError, `canceled` after cancel(). */
  done: Promise<void>;
  /** Abandons the listing and releases the backend's cursor. No-op once finished. */
  cancel(): void;
}

/**
 * Lists a directory by pulling pages until the backend returns an empty cursor. Pages are
 * requested back to back as each arrives, so the backend only reads ahead as fast as this loop.
 */
export function pullListing(
  backend: ListBackend,
  connId: string,
  dir: string,
  onPage: (entries: readonly Entry[]) => void
): Listing {
  let canceled = false;
  let finished = false;
  let inflight: Cancellable<ListPage> | null = null;
  let cursor = '';

  const release = (id: string) => {
    void Promise.resolve(backend.close(id)).catch(() => undefined);
  };

  async function run(): Promise<void> {
    let call = backend.start(connId, dir);
    for (;;) {
      inflight = call;
      let page: ListPage;
      try {
        page = await call;
      } catch (e) {
        throw canceled ? canceledError() : toAppError(e);
      }
      inflight = null;
      if (canceled) {
        // The page raced the cancel; its cursor may not have been known when cancel() ran.
        if (page.cursor) release(page.cursor);
        throw canceledError();
      }
      if (page.entries?.length) onPage(page.entries);
      if (page.cursor === '') return;
      cursor = page.cursor;
      call = backend.next(cursor);
    }
  }

  const done = run().finally(() => {
    finished = true;
  });

  return {
    done,
    cancel() {
      if (finished || canceled) return;
      canceled = true;
      if (inflight) void Promise.resolve(inflight.cancel()).catch(() => undefined);
      if (cursor) release(cursor);
    },
  };
}

function canceledError(): AppError {
  return new AppError('canceled', 'Canceled');
}
