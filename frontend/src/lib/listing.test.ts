import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Entry } from '../api/types';
import {
  applyPage,
  createPageBatcher,
  failListing,
  finishListing,
  shownEntries,
  startListing,
} from './listing';

const e = (name: string): Entry => ({
  name,
  path: `/${name}`,
  size: 1,
  modTime: '2026-01-01T00:00:00Z',
  mode: 0,
  isDir: false,
});

describe('listing accumulation', () => {
  it('collects pages and completes only when finished', () => {
    let l = startListing('r1', null);
    l = applyPage(l, 'r1', [e('a'), e('b')]);
    l = applyPage(l, 'r1', [e('c')]);
    expect(l.done).toBe(false);
    expect(l.entries.map((x) => x.name)).toEqual(['a', 'b', 'c']);
    l = finishListing(l, 'r1');
    expect(l.done).toBe(true);
  });

  it('an empty directory is complete as soon as it finishes', () => {
    expect(finishListing(startListing('r1', null), 'r1').done).toBe(true);
  });

  it('ignores pages and completion from superseded requests', () => {
    const l = startListing('r2', null);
    expect(applyPage(l, 'r1', [e('stale')])).toBe(l);
    expect(finishListing(l, 'r1')).toBe(l);
    expect(failListing(l, 'r1')).toBe(l);
  });

  it('keeps old entries visible until a refresh completes', () => {
    const old = [e('old')];
    let l = startListing('r2', old);
    l = applyPage(l, 'r2', [e('new')]);
    expect(shownEntries(l)).toBe(old);
    l = finishListing(l, 'r2');
    expect(shownEntries(l).map((x) => x.name)).toEqual(['new']);
  });

  it('a failed listing keeps the partial entries it received', () => {
    let l = applyPage(startListing('r1', null), 'r1', [e('a')]);
    l = failListing(l, 'r1');
    expect(l.done).toBe(true);
    expect(shownEntries(l).map((x) => x.name)).toEqual(['a']);
  });

  it('a failed refresh keeps showing the old entries', () => {
    const old = [e('old')];
    const l = failListing(applyPage(startListing('r2', old), 'r2', [e('new')]), 'r2');
    expect(l.done).toBe(false);
    expect(shownEntries(l)).toBe(old);
  });
});

describe('createPageBatcher', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it('merges pages per request into one flush', () => {
    const flushed: Array<Map<string, readonly Entry[]>> = [];
    const b = createPageBatcher((m) => flushed.push(new Map(m)), 50);
    b.push('r1', [e('a')]);
    b.push('r1', [e('b')]);
    b.push('r2', [e('c')]);
    expect(flushed).toHaveLength(0);
    vi.advanceTimersByTime(50);
    expect(flushed).toHaveLength(1);
    expect(flushed[0]?.get('r1')?.map((x) => x.name)).toEqual(['a', 'b']);
    expect(flushed[0]?.get('r2')?.map((x) => x.name)).toEqual(['c']);
  });

  it('flush() delivers immediately and cancels the timer', () => {
    const onFlush = vi.fn();
    const b = createPageBatcher(onFlush, 50);
    b.push('r1', [e('a')]);
    b.flush();
    expect(onFlush).toHaveBeenCalledTimes(1);
    vi.advanceTimersByTime(100);
    expect(onFlush).toHaveBeenCalledTimes(1);
  });
});
