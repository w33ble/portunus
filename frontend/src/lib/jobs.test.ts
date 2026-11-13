import { describe, expect, it } from 'vitest';
import { JobKind, State } from '../api/enums';
import type { JobView } from '../api/types';
import {
  applyUpdate,
  jobsFromList,
  newlyFinished,
  orderedJobs,
  progressFraction,
  summarize,
} from './jobs';

function job(id: string, over: Partial<JobView> = {}): JobView {
  return {
    id,
    kind: JobKind.KindCopy,
    state: State.StateRunning,
    srcConn: 'local',
    dstConn: 'local',
    dstDir: '/dst',
    title: id,
    walking: false,
    totalFiles: 10,
    doneFiles: 0,
    failedFiles: 0,
    skippedFiles: 0,
    totalBytes: 100,
    doneBytes: 0,
    bytesPerSec: 0,
    etaSeconds: -1,
    active: null,
    failed: null,
    createdAt: '2026-01-01T00:00:00Z',
    ...over,
  };
}

describe('applyUpdate', () => {
  it('upserts jobs by id', () => {
    let jobs = jobsFromList([job('a'), job('b')]);
    jobs = applyUpdate(jobs, { jobs: [job('b', { doneBytes: 50 }), job('c')], removed: null });
    expect(Object.keys(jobs).toSorted()).toEqual(['a', 'b', 'c']);
    expect(jobs['b']?.doneBytes).toBe(50);
  });

  it('drops removed jobs', () => {
    const jobs = applyUpdate(jobsFromList([job('a'), job('b')]), { jobs: null, removed: ['a'] });
    expect(Object.keys(jobs)).toEqual(['b']);
  });

  it('applies a removal and a re-add of different jobs in one update', () => {
    const jobs = applyUpdate(jobsFromList([job('a')]), { jobs: [job('z')], removed: ['a'] });
    expect(Object.keys(jobs)).toEqual(['z']);
  });

  it('returns the same map for an empty update', () => {
    const jobs = jobsFromList([job('a')]);
    expect(applyUpdate(jobs, { jobs: null, removed: ['unknown'] })).toBe(jobs);
  });
});

describe('summarize', () => {
  it('counts active jobs and sums their speed', () => {
    const jobs = jobsFromList([
      job('a', { bytesPerSec: 100 }),
      job('b', { state: State.StateWaiting, bytesPerSec: 50 }),
      job('c', { state: State.StateDone, bytesPerSec: 999 }),
      job('d', { state: State.StateFailed }),
    ]);
    expect(summarize(jobs)).toEqual({ active: 2, bytesPerSec: 150, finished: 2 });
  });
});

describe('newlyFinished', () => {
  it('reports only jobs that just left the active states', () => {
    const before = jobsFromList([job('a'), job('b', { state: State.StateDone })]);
    const after = jobsFromList([
      job('a', { state: State.StateDone }),
      job('b', { state: State.StateDone }),
      job('c', { state: State.StateFailed }),
    ]);
    expect(
      newlyFinished(before, after)
        .map((j) => j.id)
        .toSorted()
    ).toEqual(['a', 'c']);
  });
});

describe('helpers', () => {
  it('orders newest first', () => {
    const jobs = jobsFromList([
      job('old', { createdAt: '2026-01-01T00:00:00Z' }),
      job('new', { createdAt: '2026-02-01T00:00:00Z' }),
    ]);
    expect(orderedJobs(jobs).map((j) => j.id)).toEqual(['new', 'old']);
  });
  it('computes progress from bytes, else files', () => {
    expect(progressFraction(job('a', { doneBytes: 25 }))).toBe(0.25);
    expect(progressFraction(job('a', { totalBytes: 0, doneFiles: 5 }))).toBe(0.5);
    expect(progressFraction(job('a', { totalBytes: 0, totalFiles: 0 }))).toBe(0);
  });
});
