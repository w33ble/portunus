import { create } from 'zustand';
import * as api from '../api';
import { toAppError } from '../api/errors';
import { JobKind } from '../api/enums';
import type { JobView, Update } from '../api/types';
import { type JobMap, applyUpdate, jobsFromList, newlyFinished } from '../lib/jobs';
import { usePanes } from './panes';
import { useUi } from './ui';

interface TransfersState {
  jobs: JobMap;
  /** Seeds from TransferService.Jobs(); anything already received via events wins. */
  init: () => Promise<void>;
  apply: (update: Update) => void;
  cancel: (jobId: string) => Promise<void>;
  retry: (jobId: string) => Promise<void>;
  resume: (jobId: string) => Promise<void>;
  clearFinished: () => Promise<void>;
}

/**
 * Panes showing a finished job's destination need a fresh listing. A delete or move also empties
 * its source, which may be in any directory of the connection.
 */
function refreshPanesFor(job: JobView): void {
  const panes = usePanes.getState();
  switch (job.kind) {
    case JobKind.KindCopy:
      panes.refreshWhere(job.dstConn, job.dstDir);
      return;
    case JobKind.KindMove:
      panes.refreshWhere(job.srcConn);
      if (job.dstConn !== job.srcConn) panes.refreshWhere(job.dstConn, job.dstDir);
      return;
    case JobKind.KindDelete:
      panes.refreshWhere(job.srcConn);
      return;
    case JobKind.$zero:
      return;
    default: {
      const unreachable: never = job.kind;
      return unreachable;
    }
  }
}

export const useTransfers = create<TransfersState>()((set, get) => ({
  jobs: {},

  async init() {
    try {
      const snapshot = jobsFromList(await api.transfers.jobs());
      set({ jobs: { ...snapshot, ...get().jobs } });
    } catch (e) {
      useUi.getState().showError('Couldn’t load transfers', toAppError(e));
    }
  },

  apply(update) {
    const before = get().jobs;
    const after = applyUpdate(before, update);
    if (after === before) return;
    set({ jobs: after });
    for (const job of newlyFinished(before, after)) refreshPanesFor(job);
  },

  async cancel(jobId) {
    try {
      await api.transfers.cancel(jobId);
    } catch (e) {
      useUi.getState().showError('Couldn’t cancel the transfer', toAppError(e));
    }
  },

  async retry(jobId) {
    try {
      await api.transfers.retry(jobId);
    } catch (e) {
      useUi.getState().showError('Couldn’t retry the transfer', toAppError(e));
    }
  },

  async resume(jobId) {
    try {
      await api.transfers.resume(jobId);
    } catch (e) {
      useUi.getState().showError('Couldn’t resume the transfer', toAppError(e));
    }
  },

  async clearFinished() {
    try {
      await api.transfers.clear();
    } catch (e) {
      useUi.getState().showError('Couldn’t clear transfers', toAppError(e));
    }
  },
}));
