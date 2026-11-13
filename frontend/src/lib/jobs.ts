import { State } from '../api/enums';
import type { JobView, Update } from '../api/types';

export type JobMap = Readonly<Record<string, JobView>>;

export function isActiveState(state: State): boolean {
  switch (state) {
    case State.StateQueued:
    case State.StateRunning:
    case State.StateWaiting:
      return true;
    case State.$zero:
    case State.StateDone:
    case State.StateFailed:
    case State.StateCanceled:
    case State.StateSkipped:
      return false;
    default: {
      const unreachable: never = state;
      return unreachable;
    }
  }
}

export function isJobActive(job: JobView): boolean {
  return isActiveState(job.state);
}

export function jobsFromList(list: readonly JobView[]): JobMap {
  return Object.fromEntries(list.map((j) => [j.id, j]));
}

/** Upserts changed jobs and drops removed ones. Returns the same map when nothing changed. */
export function applyUpdate(jobs: JobMap, update: Update): JobMap {
  const changed = update.jobs ?? [];
  const removed = (update.removed ?? []).filter((id) => id in jobs);
  if (changed.length === 0 && removed.length === 0) return jobs;

  const next: Record<string, JobView> = { ...jobs };
  for (const id of removed) delete next[id];
  for (const job of changed) next[job.id] = job;
  return next;
}

/** Newest first. */
export function orderedJobs(jobs: JobMap): JobView[] {
  return Object.values(jobs).toSorted((a, b) => Date.parse(b.createdAt) - Date.parse(a.createdAt));
}

export interface TransferSummary {
  active: number;
  /** Sum of the running jobs' speeds. */
  bytesPerSec: number;
  finished: number;
}

export function summarize(jobs: JobMap): TransferSummary {
  let active = 0;
  let finished = 0;
  let bytesPerSec = 0;
  for (const job of Object.values(jobs)) {
    if (isJobActive(job)) {
      active++;
      bytesPerSec += job.bytesPerSec;
    } else {
      finished++;
    }
  }
  return { active, bytesPerSec, finished };
}

/** Jobs that moved from active to finished in this update, for refreshing panes. */
export function newlyFinished(before: JobMap, after: JobMap): JobView[] {
  return Object.values(after).filter((job) => {
    if (isJobActive(job)) return false;
    const prev = before[job.id];
    return prev === undefined || isJobActive(prev);
  });
}

export function progressFraction(job: JobView): number {
  if (job.totalBytes <= 0) {
    return job.totalFiles > 0 ? Math.min(1, job.doneFiles / job.totalFiles) : 0;
  }
  return Math.min(1, job.doneBytes / job.totalBytes);
}
