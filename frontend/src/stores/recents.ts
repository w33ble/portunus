import { create } from 'zustand';

const MAX_RECENTS = 8;

interface RecentsState {
  /** Folders listed on each connection this session, newest first. */
  byConn: Readonly<Record<string, readonly string[]>>;
  visit: (connId: string, path: string) => void;
}

export const useRecents = create<RecentsState>()((set) => ({
  byConn: {},
  visit: (connId, path) =>
    set((s) => {
      const prev = s.byConn[connId] ?? [];
      if (prev[0] === path) return s;
      const next = [path, ...prev.filter((p) => p !== path)].slice(0, MAX_RECENTS);
      return { byConn: { ...s.byConn, [connId]: next } };
    }),
}));
