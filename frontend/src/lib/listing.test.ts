import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Entry } from '../api/types';
import {
  applyPage,
  applyTotal,
  createPageBatcher,
  isComplete,
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
  it('completes when pages arrive before the total', () => {
    let l = startListing('r1', null);
    l = applyPage(l, 'r1', [e('a'), e('b')]);
    expect(isComplete(l)).toBe(false);
    l = applyPage(l, 'r1', [e('c')]);
    expect(isComplete(l)).toBe(false);
    l = applyTotal(l, 'r1', 3);
    expect(isComplete(l)).toBe(true);
    expect(l.entries).toHaveLength(3);
  });

  it('completes when the total arrives before the last page', () => {
    let l = startListing('r1', null);
    l = applyPage(l, 'r1', [e('a')]);
    l = applyTotal(l, 'r1', 3);
    expect(isComplete(l)).toBe(false);
    l = applyPage(l, 'r1', [e('b'), e('c')]);
    expect(isComplete(l)).toBe(true);
  });

  it('an empty directory is complete as soon as the total is 0', () => {
    expect(isComplete(applyTotal(startListing('r1', null), 'r1', 0))).toBe(true);
  });

  it('ignores pages and totals from superseded requests', () => {
    const l = startListing('r2', null);
    expect(applyPage(l, 'r1', [e('stale')])).toBe(l);
    expect(applyTotal(l, 'r1', 1)).toBe(l);
  });

  it('keeps old entries visible until a refresh completes', () => {
    const old = [e('old')];
    let l = startListing('r2', old);
    l = applyPage(l, 'r2', [e('new')]);
    expect(shownEntries(l)).toBe(old);
    l = applyTotal(l, 'r2', 1);
    expect(shownEntries(l).map((x) => x.name)).toEqual(['new']);
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
