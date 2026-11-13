import { create } from 'zustand';
import * as api from '../api';
import { toAppError } from '../api/errors';
import type { Connection, Host, S3Profile } from '../api/types';
import { useUi } from './ui';

interface ConnectionsState {
  connections: readonly Connection[];
  hosts: readonly Host[];
  profiles: readonly S3Profile[];
  /** Non-empty when SSH can't be used, e.g. no ssh binary. */
  sshUnavailable: string;
  /** True when saved S3 secrets last only for this session (no OS keychain). */
  secretsSessionOnly: boolean;
  load: () => Promise<void>;
  setConnections: (list: readonly Connection[]) => void;
  upsert: (conn: Connection) => void;
  saveProfile: (profile: S3Profile, secret: string) => Promise<S3Profile>;
  deleteProfile: (id: string) => Promise<void>;
}

export const useConnections = create<ConnectionsState>()((set, get) => ({
  connections: [],
  hosts: [],
  profiles: [],
  sshUnavailable: '',
  secretsSessionOnly: false,

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
  },
}));
