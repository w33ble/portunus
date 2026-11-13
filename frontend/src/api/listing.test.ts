import { describe, expect, it, vi } from 'vitest';
import { bindingError, deferred, entry } from '../stores/testApi';
import { AppError } from './errors';
import { type Cancellable, type ListBackend, pullListing } from './listing';
import type { ListPage } from './types';

type Call = ReturnType<typeof deferred<ListPage>> & { cancel: ReturnType<typeof vi.fn> };

/** A backend whose every call is a controllable promise, in call order. */
function fakeBackend() {
  const calls: Call[] = [];
  const make = (): Cancellable<ListPage> => {
    const d = deferred<ListPage>();
    const call = Object.assign(d, { cancel: vi.fn() });
    calls.push(call);
    return Object.assign(d.promise, { cancel: call.cancel });
  };
  const backend = {
    start: vi.fn(make),
    next: vi.fn(make),
    close: vi.fn(() => Promise.resolve()),
  } satisfies ListBackend;
  return { backend, calls };
}

const page = (cursor: string, ...names: string[]): ListPage => ({
  entries: names.map((n) => entry(n)),
  cursor,
});
const names = (batches: unknown[][]) =>
  batches.map((b) => (b as { name: string }[]).map((e) => e.name));
const tick = () => new Promise((r) => setTimeout(r, 0));

describe('pullListing', () => {
  it('delivers each page as it arrives and requests the next with the returned cursor', async () => {
    const { backend, calls } = fakeBackend();
    const batches: unknown[][] = [];
    const listing = pullListing(backend, 'c1', '/d', (es) => batches.push([...es]));
    expect(backend.start).toHaveBeenCalledWith('c1', '/d');
    expect(backend.next).not.toHaveBeenCalled();

    calls[0]!.resolve(page('cur', 'a', 'b'));
    await tick();
    expect(names(batches)).toEqual([['a', 'b']]);
    expect(backend.next).toHaveBeenCalledWith('cur');

    calls[1]!.resolve(page('cur', 'c'));
    await tick();
    expect(names(batches)).toEqual([['a', 'b'], ['c']]);

    calls[2]!.resolve(page('', 'd'));
    await listing.done;
    expect(names(batches)).toEqual([['a', 'b'], ['c'], ['d']]);
    expect(backend.next).toHaveBeenCalledTimes(2);
    expect(backend.close).not.toHaveBeenCalled();
  });

  it('finishes an empty directory without delivering a page', async () => {
    const { backend, calls } = fakeBackend();
    const onPage = vi.fn();
    const listing = pullListing(backend, 'c1', '/d', onPage);
    calls[0]!.resolve({ entries: null as never, cursor: '' });
    await listing.done;
    expect(onPage).not.toHaveBeenCalled();
  });

  it('rejects with the typed error when a later page fails, keeping earlier pages', async () => {
    const { backend, calls } = fakeBackend();
    const onPage = vi.fn();
    const listing = pullListing(backend, 'c1', '/d', onPage);
    calls[0]!.resolve(page('cur', 'a'));
    await tick();
    calls[1]!.reject(bindingError('permission', 'denied'));
    await expect(listing.done).rejects.toMatchObject({ kind: 'permission', message: 'denied' });
    expect(onPage).toHaveBeenCalledTimes(1);
    expect(backend.close).not.toHaveBeenCalled(); // the backend already ended it
  });

  it('rejects with the typed error when the first call fails', async () => {
    const { backend, calls } = fakeBackend();
    const listing = pullListing(backend, 'c1', '/d', vi.fn());
    calls[0]!.reject(bindingError('not_connected', 'ssh:h: not connected'));
    await expect(listing.done).rejects.toMatchObject({ kind: 'not_connected' });
  });

  it('cancel aborts the in-flight call, closes the cursor, and rejects as canceled', async () => {
    const { backend, calls } = fakeBackend();
    const onPage = vi.fn();
    const listing = pullListing(backend, 'c1', '/d', onPage);
    calls[0]!.resolve(page('cur', 'a'));
    await tick();
    expect(onPage).toHaveBeenCalledTimes(1);

    listing.cancel();
    expect(calls[1]!.cancel).toHaveBeenCalled();
    expect(backend.close).toHaveBeenCalledWith('cur');

    const cancelled = new Error('Promise cancelled.');
    cancelled.name = 'CancelError';
    calls[1]!.reject(cancelled);
    const err = await listing.done.catch((e: unknown) => e);
    expect(err).toBeInstanceOf(AppError);
    expect(err).toMatchObject({ kind: 'canceled' });
  });

  it('cancel before the first page aborts the call', async () => {
    const { backend, calls } = fakeBackend();
    const listing = pullListing(backend, 'c1', '/d', vi.fn());
    listing.cancel();
    expect(calls[0]!.cancel).toHaveBeenCalled();
    expect(backend.close).not.toHaveBeenCalled(); // no cursor yet
    calls[0]!.reject(new Error('Promise cancelled.'));
    await expect(listing.done).rejects.toMatchObject({ kind: 'canceled' });
  });

  it('closes the cursor of a page that races the cancel and delivers nothing', async () => {
    const { backend, calls } = fakeBackend();
    const onPage = vi.fn();
    const listing = pullListing(backend, 'c1', '/d', onPage);
    listing.cancel();
    calls[0]!.resolve(page('late', 'a'));
    await expect(listing.done).rejects.toMatchObject({ kind: 'canceled' });
    expect(onPage).not.toHaveBeenCalled();
    expect(backend.close).toHaveBeenCalledWith('late');
    expect(backend.next).not.toHaveBeenCalled();
  });

  it('cancel after the listing finished does nothing', async () => {
    const { backend, calls } = fakeBackend();
    const listing = pullListing(backend, 'c1', '/d', vi.fn());
    calls[0]!.resolve(page('', 'a'));
    await listing.done;
    listing.cancel();
    expect(backend.close).not.toHaveBeenCalled();
  });

  it('swallows a failing close', async () => {
    const { backend, calls } = fakeBackend();
    backend.close.mockRejectedValue(new Error('gone'));
    const listing = pullListing(backend, 'c1', '/d', vi.fn());
    calls[0]!.resolve(page('cur', 'a'));
    await tick();
    listing.cancel();
    calls[1]!.reject(new Error('Promise cancelled.'));
    await expect(listing.done).rejects.toMatchObject({ kind: 'canceled' });
  });
});
