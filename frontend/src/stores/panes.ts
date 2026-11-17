import { create } from 'zustand';
import * as api from '../api';
import { type AppError, isCanceled, toAppError } from '../api/errors';
import type { Entry, Target } from '../api/types';
import {
  EMPTY_LISTING,
  type Listing,
  applyPage,
  createPageBatcher,
  failListing,
  finishListing,
  shownEntries,
  startListing,
} from '../lib/listing';
import { EMPTY_HISTORY, type History, type Place, back, forward, visit } from '../lib/history';
import { normalizePath, parentPath } from '../lib/paths';
import { EMPTY_SELECTION, type Selection } from '../lib/selection';
import { DEFAULT_SORT, type SortKey, type SortState, toggleSort } from '../lib/sort';
import { useConnections } from './connections';
import { PANE_IDS, type PaneId } from './paneTypes';
import { useRecents } from './recents';
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
  /** Back/forward stacks of folders visited on this connection. */
  history: History;
  /** Filter query; null when the filter field is closed. Cleared on navigation. */
  filter: string | null;
}

type PaneMap = Readonly<Record<PaneId, PaneState>>;

interface PanesState {
  panes: PaneMap;
  /** Opens both panes on the local home directory. No-op once a pane is in use. */
  init: () => void;
  /**
   * Connects the pane to `target` and lists `path`, else the connection's open-at folder,
   * else its home. Resolves true once connected.
   */
  connectTo: (id: PaneId, target: Target, label: string, path?: string) => Promise<boolean>;
  /** Redials the pane's last target and relists the current directory. */
  reconnect: (id: PaneId) => Promise<void>;
  navigate: (id: PaneId, path: string) => void;
  goUp: (id: PaneId) => void;
  /** Returns to the previous folder in this pane's history. */
  goBack: (id: PaneId) => void;
  goForward: (id: PaneId) => void;
  /** Relists this pane's directory, and the other pane too when it shows the same one. */
  refresh: (id: PaneId) => void;
  /** Refreshes panes showing `dir` on `connId` (or any directory of the connection when omitted). */
  refreshWhere: (connId: string, dir?: string) => void;
  applyPages: (batch: ReadonlyMap<string, readonly Entry[]>) => void;
  setSelection: (id: PaneId, selection: Selection) => void;
  toggleSort: (id: PaneId, key: SortKey) => void;
  /** Shows the filter field, keeping an existing query. */
  openFilter: (id: PaneId) => void;
  setFilter: (id: PaneId, query: string) => void;
  closeFilter: (id: PaneId) => void;
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
    history: EMPTY_HISTORY,
    filter: null,
  };
}

const initialPanes: PaneMap = { left: blankPane('left'), right: blankPane('right') };

/** Wire-level handles aren't state: they're only cancelled and compared. */
const inflight: Partial<Record<PaneId, api.Listing>> = {};
const connectSeq: Record<PaneId, number> = { left: 0, right: 0 };
let requestCounter = 0;

/** Pane in loading state turns ready exactly when its listing is complete. */
function settle(pane: PaneState): PaneState {
  return pane.status.kind === 'loading' && pane.listing.done
    ? { ...pane, status: { kind: 'ready' } }
    : pane;
}

const PAGE_FLUSH_MS = 60;

/** Where the pane is now, as a history entry. */
const here = (pane: PaneState): Place => ({ path: pane.path, selection: pane.selection });

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
    opts: {
      lastTarget?: PaneState['lastTarget'];
      history?: History;
      restore?: Selection;
      /** Listed instead when `path` doesn't exist, e.g. a stale open-at folder. */
      fallback?: string;
    } = {}
  ) {
    const requestId = `${id}-${++requestCounter}`;
    inflight[id]?.cancel();

    patch(id, (pane) => ({
      ...pane,
      connId,
      lastTarget: opts.lastTarget ?? pane.lastTarget,
      path,
      history: opts.history ?? pane.history,
      status: { kind: 'loading' },
      listing: startListing(requestId, mode === 'refresh' ? shownEntries(pane.listing) : null),
      selection: mode === 'refresh' ? pane.selection : EMPTY_SELECTION,
      filter: mode === 'refresh' ? pane.filter : null,
    }));

    const handle = api.files.list(connId, path, (entries) => pageBatcher.push(requestId, entries));
    inflight[id] = handle;

    handle.done.then(
      () => {
        if (!isCurrent(id, requestId)) return;
        useRecents.getState().visit(connId, path);
        pageBatcher.flush();
        patch(id, (pane) =>
          settle({
            ...pane,
            listing: finishListing(pane.listing, requestId),
            // Back/forward reselect what was selected there, unless the user already picked.
            selection:
              opts.restore && pane.selection === EMPTY_SELECTION ? opts.restore : pane.selection,
          })
        );
      },
      (e: unknown) => {
        if (isCanceled(e) || !isCurrent(id, requestId)) return;
        const error = toAppError(e);
        if (opts.fallback !== undefined && error.kind === 'not_exist') {
          const { fallback, ...rest } = opts;
          load(id, connId, fallback, mode, rest);
          return;
        }
        pageBatcher.flush();
        patch(id, (pane) => ({
          ...pane,
          status: { kind: 'error', error },
          listing: failListing(pane.listing, requestId),
        }));
      }
    );
  }

  /**
   * Connects `target`; lists `keepPath` when reconnecting in place, else `path`, the
   * connection's open-at folder, or its home.
   */
  async function dial(
    id: PaneId,
    target: Target,
    label: string,
    keepPath: string | null,
    path?: string
  ): Promise<boolean> {
    const token = ++connectSeq[id];
    patch(id, (pane) => ({ ...pane, status: { kind: 'connecting', label } }));
    try {
      const conn = await api.connections.connect(target);
      if (token !== connectSeq[id]) return false;
      useConnections.getState().upsert(conn);
      const home = normalizePath(conn.home || '/');
      const openAt = useConnections.getState().prefs[conn.id]?.openAt;
      const to = keepPath ?? normalizePath(path ?? openAt ?? home);
      // A new connection starts a fresh history; reconnecting in place keeps it.
      load(id, conn.id, to, keepPath === null ? 'navigate' : 'refresh', {
        lastTarget: { target, label },
        ...(keepPath === null && { history: EMPTY_HISTORY }),
        ...(keepPath === null && path === undefined && to !== home && { fallback: home }),
      });
      return true;
    } catch (e) {
      if (token !== connectSeq[id]) return false;
      const error = toAppError(e);
      patch(id, (pane) => ({ ...pane, status: { kind: 'error', error } }));
      useUi.getState().showError(`Couldn’t connect to ${label}`, error);
      return false;
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

    connectTo: (id, target, label, path) => dial(id, target, label, null, path),

    async reconnect(id) {
      const { lastTarget, path } = get().panes[id];
      if (lastTarget !== null) await dial(id, lastTarget.target, lastTarget.label, path);
    },

    navigate(id, path) {
      const pane = get().panes[id];
      if (pane.connId === null) return;
      const to = normalizePath(path);
      load(id, pane.connId, to, 'navigate', { history: visit(pane.history, here(pane), to) });
    },

    goUp(id) {
      const pane = get().panes[id];
      const parent = parentPath(pane.path);
      if (pane.connId === null || parent === pane.path) return;
      load(id, pane.connId, parent, 'navigate', {
        history: visit(pane.history, here(pane), parent),
      });
    },

    goBack(id) {
      const pane = get().panes[id];
      const step = pane.connId === null ? null : back(pane.history, here(pane));
      if (pane.connId !== null && step) {
        load(id, pane.connId, step.place.path, 'navigate', {
          history: step.history,
          restore: step.place.selection,
        });
      }
    },

    goForward(id) {
      const pane = get().panes[id];
      const step = pane.connId === null ? null : forward(pane.history, here(pane));
      if (pane.connId !== null && step) {
        load(id, pane.connId, step.place.path, 'navigate', {
          history: step.history,
          restore: step.place.selection,
        });
      }
    },

    refresh(id) {
      const { connId, path } = get().panes[id];
      if (connId !== null) get().refreshWhere(connId, path);
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
          const listing = applyPage(pane.listing, pane.listing.requestId, entries);
          next = { ...next, [id]: { ...pane, listing } };
        }
        return next === s.panes ? s : { panes: next };
      });
    },

    setSelection: (id, selection) => patch(id, (pane) => ({ ...pane, selection })),

    toggleSort: (id, key) => patch(id, (pane) => ({ ...pane, sort: toggleSort(pane.sort, key) })),

    openFilter: (id) => patch(id, (pane) => ({ ...pane, filter: pane.filter ?? '' })),
    setFilter: (id, query) => patch(id, (pane) => ({ ...pane, filter: query })),
    closeFilter: (id) => patch(id, (pane) => ({ ...pane, filter: null })),

    dismissError(id) {
      patch(id, (pane) =>
        pane.status.kind === 'error'
          ? { ...pane, status: pane.listing.done ? { kind: 'ready' } : { kind: 'idle' } }
          : pane
      );
    },
  };
});

const pageBatcher = createPageBatcher(
  (batch) => usePanes.getState().applyPages(batch),
  PAGE_FLUSH_MS
);

function localTarget(): Target {
  return api.targets.local();
}
