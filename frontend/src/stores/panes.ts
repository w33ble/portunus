import { create } from 'zustand';
import * as api from '../api';
import { type AppError, isCanceled, toAppError } from '../api/errors';
import type { Entry, Target } from '../api/types';
import {
  EMPTY_LISTING,
  type Listing,
  applyPage,
  applyTotal,
  createPageBatcher,
  isComplete,
  shownEntries,
  startListing,
} from '../lib/listing';
import { normalizePath, parentPath } from '../lib/paths';
import { EMPTY_SELECTION, type Selection } from '../lib/selection';
import { DEFAULT_SORT, type SortKey, type SortState, toggleSort } from '../lib/sort';
import { useConnections } from './connections';
import { PANE_IDS, type PaneId } from './paneTypes';
import { useUi } from './ui';

export type PaneStatus =
  | { kind: 'idle' }
  | { kind: 'connecting'; label: string }
  | { kind: 'loading' }
  | { kind: 'ready' }
  | { kind: 'error'; error: AppError };

export interface PaneState {
  id: PaneId;
  connId: string | null;
  /** What `connId` was dialed from, kept so a lost connection can be reopened. */
  lastTarget: { target: Target; label: string } | null;
  path: string;
  status: PaneStatus;
  listing: Listing;
  selection: Selection;
  sort: SortState;
}

type PaneMap = Readonly<Record<PaneId, PaneState>>;

interface PanesState {
  panes: PaneMap;
  /** Opens both panes on the local home directory. No-op once a pane is in use. */
  init: () => void;
  connectTo: (id: PaneId, target: Target, label: string) => Promise<void>;
  /** Redials the pane's last target and relists the current directory. */
  reconnect: (id: PaneId) => Promise<void>;
  navigate: (id: PaneId, path: string) => void;
  goUp: (id: PaneId) => void;
  refresh: (id: PaneId) => void;
  /** Refreshes panes showing `dir` on `connId` (or any directory of the connection when omitted). */
  refreshWhere: (connId: string, dir?: string) => void;
  applyPages: (batch: ReadonlyMap<string, readonly Entry[]>) => void;
  setSelection: (id: PaneId, selection: Selection) => void;
  toggleSort: (id: PaneId, key: SortKey) => void;
  dismissError: (id: PaneId) => void;
}

function blankPane(id: PaneId): PaneState {
  return {
    id,
    connId: null,
    lastTarget: null,
    path: '/',
    status: { kind: 'idle' },
    listing: EMPTY_LISTING,
    selection: EMPTY_SELECTION,
    sort: DEFAULT_SORT,
  };
}

const initialPanes: PaneMap = { left: blankPane('left'), right: blankPane('right') };

/** Wire-level handles aren't state: they're only cancelled and compared. */
const inflight: Partial<Record<PaneId, api.Listing>> = {};
const connectSeq: Record<PaneId, number> = { left: 0, right: 0 };
let requestCounter = 0;

/** Pane in loading state turns ready exactly when its listing is complete. */
function settle(pane: PaneState): PaneState {
  return pane.status.kind === 'loading' && isComplete(pane.listing)
    ? { ...pane, status: { kind: 'ready' } }
    : pane;
}

const PAGE_FLUSH_MS = 60;

export const usePanes = create<PanesState>()((set, get) => {
  const patch = (id: PaneId, update: (pane: PaneState) => PaneState) =>
    set((s) => ({ panes: { ...s.panes, [id]: update(s.panes[id]) } }));

  const isCurrent = (id: PaneId, requestId: string) =>
    get().panes[id].listing.requestId === requestId;

  function load(
    id: PaneId,
    connId: string,
    path: string,
    mode: 'navigate' | 'refresh',
    lastTarget?: PaneState['lastTarget']
  ) {
    const requestId = `${id}-${++requestCounter}`;
    inflight[id]?.cancel();

    patch(id, (pane) => ({
      ...pane,
      connId,
      lastTarget: lastTarget ?? pane.lastTarget,
      path,
      status: { kind: 'loading' },
      listing: startListing(requestId, mode === 'refresh' ? shownEntries(pane.listing) : null),
      selection: mode === 'refresh' ? pane.selection : EMPTY_SELECTION,
    }));

    const handle = api.files.list(connId, path, requestId);
    inflight[id] = handle;

    handle.total.then(
      (total) => {
        if (!isCurrent(id, requestId)) return;
        pageBatcher.flush();
        patch(id, (pane) =>
          settle({ ...pane, listing: applyTotal(pane.listing, requestId, total) })
        );
      },
      (e: unknown) => {
        if (isCanceled(e) || !isCurrent(id, requestId)) return;
        patch(id, (pane) => ({
          ...pane,
          status: { kind: 'error', error: toAppError(e) },
          listing:
            pane.listing.replacing === null
              ? { ...pane.listing, total: pane.listing.entries.length }
              : pane.listing,
        }));
      }
    );
  }

  /** Connects `target`; lists `keepPath` when given, else the connection's home. */
  async function dial(id: PaneId, target: Target, label: string, keepPath: string | null) {
    const token = ++connectSeq[id];
    patch(id, (pane) => ({ ...pane, status: { kind: 'connecting', label } }));
    try {
      const conn = await api.connections.connect(target);
      if (token !== connectSeq[id]) return;
      useConnections.getState().upsert(conn);
      const path = keepPath ?? normalizePath(conn.home || '/');
      load(id, conn.id, path, keepPath === null ? 'navigate' : 'refresh', { target, label });
    } catch (e) {
      if (token !== connectSeq[id]) return;
      const error = toAppError(e);
      patch(id, (pane) => ({ ...pane, status: { kind: 'error', error } }));
      useUi.getState().showError(`Couldn’t connect to ${label}`, error);
    }
  }

  return {
    panes: initialPanes,

    init() {
      for (const id of PANE_IDS) {
        const pane = get().panes[id];
        if (pane.connId === null && pane.status.kind === 'idle') {
          void get().connectTo(id, localTarget(), 'Local');
        }
      }
    },

    async connectTo(id, target, label) {
      await dial(id, target, label, null);
    },

    async reconnect(id) {
      const { lastTarget, path } = get().panes[id];
      if (lastTarget !== null) await dial(id, lastTarget.target, lastTarget.label, path);
    },

    navigate(id, path) {
      const { connId } = get().panes[id];
      if (connId !== null) load(id, connId, normalizePath(path), 'navigate');
    },

    goUp(id) {
      const { connId, path } = get().panes[id];
      const parent = parentPath(path);
      if (connId !== null && parent !== path) load(id, connId, parent, 'navigate');
    },

    refresh(id) {
      const { connId, path } = get().panes[id];
      if (connId !== null) load(id, connId, path, 'refresh');
    },

    refreshWhere(connId, dir) {
      for (const id of PANE_IDS) {
        const pane = get().panes[id];
        if (pane.connId === connId && (dir === undefined || pane.path === normalizePath(dir))) {
          load(id, connId, pane.path, 'refresh');
        }
      }
    },

    applyPages(batch) {
      set((s) => {
        let next = s.panes;
        for (const id of PANE_IDS) {
          const pane = next[id];
          const entries = batch.get(pane.listing.requestId);
          if (entries === undefined) continue;
          const updated = settle({
            ...pane,
            listing: applyPage(pane.listing, pane.listing.requestId, entries),
          });
          next = { ...next, [id]: updated };
        }
        return next === s.panes ? s : { panes: next };
      });
    },

    setSelection: (id, selection) => patch(id, (pane) => ({ ...pane, selection })),

    toggleSort: (id, key) => patch(id, (pane) => ({ ...pane, sort: toggleSort(pane.sort, key) })),

    dismissError(id) {
      patch(id, (pane) =>
        pane.status.kind === 'error'
          ? { ...pane, status: isComplete(pane.listing) ? { kind: 'ready' } : { kind: 'idle' } }
          : pane
      );
    },
  };
});

const pageBatcher = createPageBatcher(
  (batch) => usePanes.getState().applyPages(batch),
  PAGE_FLUSH_MS
);

/** Entry point for `files:page` events. */
export function receivePage(requestId: string, entries: readonly Entry[]): void {
  pageBatcher.push(requestId, entries);
}

function localTarget(): Target {
  return api.targets.local();
}
