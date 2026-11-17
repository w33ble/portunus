import { useMemo } from 'react';
import { create } from 'zustand';
import { useShallow } from 'zustand/react/shallow';
import * as api from '../api';
import { toAppError } from '../api/errors';
import type { Bookmark, Connection, Host, Prefs, S3Profile, SavedHost } from '../api/types';
import { type KnownConnection, buildCatalog } from '../lib/catalog';
import { selectShowHidden, useSettings } from './settings';
import { useUi } from './ui';

interface ConnectionsState {
  connections: readonly Connection[];
  hosts: readonly Host[];
  profiles: readonly S3Profile[];
  /** SSH hosts saved in the app, as opposed to ~/.ssh/config aliases. */
  savedHosts: readonly SavedHost[];
  /** Per-connection preferences, by connection ID. */
  prefs: Partial<Record<string, Prefs>>;
  bookmarks: readonly Bookmark[];
  /** Non-empty when SSH can't be used, e.g. no ssh binary. */
  sshUnavailable: string;
  /** True when saved S3 secrets last only for this session (no OS keychain). */
  secretsSessionOnly: boolean;
  /** Bumped each time ~/.ssh/config is read again, so views that resolved an alias redo it. */
  hostsVersion: number;
  load: () => Promise<void>;
  /** Reloads saved hosts, preferences and bookmarks. */
  loadSaved: () => Promise<void>;
  /** Reads ~/.ssh/config again. Rejects, keeping the old host list, when it can't be read. */
  reloadHosts: () => Promise<void>;
  setConnections: (list: readonly Connection[]) => void;
  upsert: (conn: Connection) => void;
  saveProfile: (profile: S3Profile, secret: string) => Promise<S3Profile>;
  deleteProfile: (id: string) => Promise<void>;
  saveHost: (host: SavedHost) => Promise<SavedHost>;
  deleteHost: (id: string) => Promise<void>;
  /** Replaces one connection's preferences and bookmarks. */
  savePrefs: (connId: string, prefs: Prefs, bookmarks: Bookmark[]) => Promise<void>;
  addBookmark: (connId: string, path: string) => Promise<Bookmark>;
  removeBookmark: (id: string) => Promise<void>;
}

export const useConnections = create<ConnectionsState>()((set, get) => ({
  connections: [],
  hosts: [],
  profiles: [],
  savedHosts: [],
  prefs: {},
  bookmarks: [],
  sshUnavailable: '',
  secretsSessionOnly: false,
  hostsVersion: 0,

  async load() {
    const [connections, hosts, profiles, sshUnavailable, secretsSessionOnly] =
      await Promise.allSettled([
        api.connections.list(),
        api.connections.sshHosts(),
        api.connections.s3Profiles(),
        api.connections.sshAvailable(),
        api.connections.secretsSessionOnly(),
      ]);
    set({
      connections: connections.status === 'fulfilled' ? connections.value : get().connections,
      hosts: hosts.status === 'fulfilled' ? hosts.value : [],
      profiles: profiles.status === 'fulfilled' ? profiles.value : [],
      sshUnavailable: sshUnavailable.status === 'fulfilled' ? sshUnavailable.value : '',
      secretsSessionOnly:
        secretsSessionOnly.status === 'fulfilled' ? secretsSessionOnly.value : false,
    });
    if (hosts.status === 'rejected') {
      useUi.getState().showError('Couldn’t read your SSH config', toAppError(hosts.reason));
    }
    // After the host list: loading saved data drops preferences of aliases that are gone.
    await get().loadSaved();
  },

  async loadSaved() {
    try {
      const saved = await api.connections.saved();
      set({ savedHosts: saved.hosts, prefs: saved.prefs, bookmarks: saved.bookmarks });
    } catch (e) {
      useUi.getState().showError('Couldn’t read saved connections', toAppError(e));
    }
  },

  async reloadHosts() {
    const [hosts, sshUnavailable] = await Promise.all([
      api.connections.sshHosts(),
      api.connections.sshAvailable().catch(() => get().sshUnavailable),
    ]);
    set((s) => ({ hosts, sshUnavailable, hostsVersion: s.hostsVersion + 1 }));
    // Drops preferences of aliases that left the config.
    await get().loadSaved();
  },

  setConnections: (connections) => set({ connections }),

  upsert(conn) {
    const rest = get().connections.filter((c) => c.id !== conn.id);
    set({ connections: [...rest, conn] });
  },

  async saveProfile(profile, secret) {
    const saved = await api.connections.saveS3Profile(profile, secret);
    const [profiles, sessionOnly] = await Promise.all([
      api.connections.s3Profiles(),
      api.connections.secretsSessionOnly().catch(() => get().secretsSessionOnly),
    ]);
    set({ profiles, secretsSessionOnly: sessionOnly });
    return saved;
  },

  async deleteProfile(id) {
    await api.connections.deleteS3Profile(id);
    set({ profiles: await api.connections.s3Profiles() });
    await get().loadSaved();
  },

  async saveHost(host) {
    const saved = await api.connections.saveSSHHost(host);
    await get().loadSaved();
    return saved;
  },

  async deleteHost(id) {
    await api.connections.deleteSSHHost(id);
    await get().loadSaved();
  },

  async savePrefs(connId, prefs, bookmarks) {
    await api.connections.saveConnectionPrefs(connId, prefs, bookmarks);
    await get().loadSaved();
  },

  async addBookmark(connId, path) {
    const bookmark = await api.connections.addBookmark({ id: '', connId, path });
    await get().loadSaved();
    return bookmark;
  },

  async removeBookmark(id) {
    await api.connections.removeBookmark(id);
    await get().loadSaved();
  },
}));

/** Every known connection, with its preferences, in display order. */
export function useCatalog(): readonly KnownConnection[] {
  const input = useConnections(
    useShallow((s) => ({
      hosts: s.hosts,
      savedHosts: s.savedHosts,
      profiles: s.profiles,
      prefs: s.prefs,
    }))
  );
  return useMemo(() => buildCatalog(input), [input]);
}

/** The catalog right now, for event handlers and actions. */
export function catalogNow(): KnownConnection[] {
  return buildCatalog(useConnections.getState());
}

/** Whether a connection lists dotfiles: its own setting when set, else the global one. */
export function useShowHidden(connId: string | null): boolean {
  const own = useConnections((s) => (connId === null ? undefined : s.prefs[connId]?.showHidden));
  const global = useSettings(selectShowHidden);
  return own ?? global;
}

export function showHiddenNow(connId: string | null): boolean {
  const own = connId === null ? undefined : useConnections.getState().prefs[connId]?.showHidden;
  return own ?? selectShowHidden(useSettings.getState());
}
