// Stands in for src/api/index.ts in the demo build (see vite.config.ts): the same surface,
// answered from the fixtures in data.ts instead of the Wails bindings.
import { FileMode } from '../bindings/io/fs/models';
import { AppError } from '../src/api/errors';
import { loadPreview } from '../src/api/preview';
import type * as real from '../src/api';
import type {
  Bookmark,
  Connection,
  Entry,
  Prefs,
  S3Profile,
  SavedHost,
  Target,
} from '../src/api/types';
import { TargetKind } from '../src/api/enums';
import { isJobActive } from '../src/lib/jobs';
import {
  BOOKMARKS,
  INITIAL_LIVE,
  JOBS,
  MACHINES,
  PREFS,
  S3_PROFILES,
  SAVED_HOSTS,
  SETTINGS,
  SSH_DETAILS,
  SSH_HOSTS,
  VERSION,
} from './data';
import { DAY, ago } from './trees';

export { AppError, isCanceled, toAppError } from '../src/api/errors';
export { targets } from '../src/api/targets';
export type { Listing, NoPreviewReason, PreviewContent, Saved } from '../src/api';

const live = new Set(INITIAL_LIVE);
let prefs: Record<string, Prefs> = { ...PREFS };
let bookmarks: Bookmark[] = [...BOOKMARKS];
let hosts: SavedHost[] = [...SAVED_HOSTS];
let profiles: S3Profile[] = [...S3_PROFILES];
let stored = { ...SETTINGS };
let nextId = 0;

const newId = (prefix: string) => `${prefix}-${++nextId}`;

function targetId(target: Target): string {
  switch (target.kind) {
    case TargetKind.TargetLocal:
      return 'local';
    case TargetKind.TargetS3:
      return `s3:${target.profileId ?? ''}`;
    case TargetKind.TargetSSH: {
      if (target.alias) return `ssh:${target.alias}`;
      const user = target.user ? `${target.user}@` : '';
      return `ssh:${user}${target.host ?? ''}${target.port ? `:${target.port}` : ''}`;
    }
    case TargetKind.$zero:
      throw new AppError('other', 'no connection target');
    default: {
      const unreachable: never = target.kind;
      return unreachable;
    }
  }
}

function machine(connId: string) {
  const found = MACHINES.get(connId);
  if (!found) throw new AppError('not_connected', `${connId} isn't connected`);
  return found;
}

const folder = (path: string): Entry => ({
  name: path.slice(path.lastIndexOf('/') + 1),
  path,
  size: 0,
  modTime: ago(DAY),
  mode: FileMode.ModeDir | 0o755,
  isDir: true,
});

/** A directory's entries; folders above the fixtures' own listings appear as bare folders. */
function readDir(connId: string, dir: string): Entry[] {
  const { tree } = machine(connId);
  const own = tree.get(dir);
  if (own) return own;
  const base = dir === '/' ? '/' : `${dir}/`;
  const children = new Set<string>();
  for (const key of tree.keys()) {
    if (key.startsWith(base) && key.length > base.length) {
      children.add(base + key.slice(base.length).split('/', 1)[0]);
    }
  }
  if (children.size === 0) throw new AppError('not_exist', `${dir}: no such file or directory`);
  return [...children].map(folder);
}

function writeDir(connId: string, dir: string, list: Entry[]) {
  machine(connId).tree.set(dir, list);
}

export const files: typeof real.files = {
  list(connId, dir, onPage) {
    let canceled = false;
    const done = (async () => {
      await Promise.resolve();
      if (canceled) throw new AppError('canceled', 'Canceled');
      const entries = readDir(connId, dir);
      if (entries.length > 0) onPage(entries);
    })();
    return {
      done,
      cancel() {
        canceled = true;
      },
    };
  },
  async mkdir(connId, dir, name) {
    const entry = { ...folder(`${dir === '/' ? '' : dir}/${name}`), modTime: ago(0) };
    writeDir(connId, dir, [...readDir(connId, dir), entry]);
    return entry;
  },
  async rename(connId, path, newName) {
    const dir = path.slice(0, path.lastIndexOf('/')) || '/';
    const list = readDir(connId, dir);
    const old = list.find((e) => e.path === path);
    if (!old) throw new AppError('not_exist', `${path}: no such file or directory`);
    const renamed = { ...old, name: newName, path: `${dir === '/' ? '' : dir}/${newName}` };
    writeDir(
      connId,
      dir,
      list.map((e) => (e === old ? renamed : e))
    );
    return renamed;
  },
  async summarize(connId, dir) {
    const list = readDir(connId, dir);
    const dirs = list.filter((e) => e.isDir).length;
    const size = list.reduce((sum, e) => sum + e.size, 0);
    return { dirs, files: list.length - dirs, size, partial: false };
  },
};

export const preview: typeof real.preview = { load: loadPreview };

const liveConnections = (): Connection[] =>
  [...live].flatMap((id) => MACHINES.get(id)?.connection ?? []);

export const connections: typeof real.connections = {
  list: async () => liveConnections(),
  async connect(target) {
    const id = targetId(target);
    const found = MACHINES.get(id);
    if (!found) throw new AppError('other', `Couldn’t resolve ${id.replace(/^ssh:/, '')}`);
    live.add(id);
    return found.connection;
  },
  sshHosts: async () => [...SSH_HOSTS],
  sshAvailable: async () => '',
  secretsSessionOnly: async () => false,
  s3Profiles: async () => [...profiles],
  async saveS3Profile(profile) {
    const saved = { ...profile, id: profile.id || newId('s3') };
    profiles = [...profiles.filter((p) => p.id !== saved.id), saved];
    return saved;
  },
  async deleteS3Profile(id) {
    profiles = profiles.filter((p) => p.id !== id);
    delete prefs[`s3:${id}`];
    bookmarks = bookmarks.filter((b) => b.connId !== `s3:${id}`);
  },
  async sshHostDetails(alias) {
    const details = SSH_DETAILS[alias];
    if (!details) throw new AppError('not_exist', `${alias} isn't in ~/.ssh/config`);
    return details;
  },
  saved: async () => ({
    hosts: [...hosts],
    prefs: structuredClone(prefs),
    bookmarks: structuredClone(bookmarks),
  }),
  async saveSSHHost(host) {
    const saved = { ...host, id: host.id || newId('host') };
    hosts = [...hosts.filter((h) => h.id !== saved.id), saved];
    return saved;
  },
  async deleteSSHHost(id) {
    hosts = hosts.filter((h) => h.id !== id);
  },
  async saveConnectionPrefs(connId, next, list) {
    prefs = { ...prefs, [connId]: next };
    const kept = bookmarks.filter((b) => b.connId !== connId);
    bookmarks = [...kept, ...list.map((b) => ({ ...b, id: b.id || newId('bm') }))];
  },
  async addBookmark(bookmark) {
    const added = { ...bookmark, id: newId('bm') };
    bookmarks = [...bookmarks, added];
    return added;
  },
  async removeBookmark(id) {
    const gone = bookmarks.find((b) => b.id === id);
    bookmarks = bookmarks.filter((b) => b.id !== id);
    const own = gone ? prefs[gone.connId] : undefined;
    if (gone && own?.openAt === gone.path)
      prefs = { ...prefs, [gone.connId]: { ...own, openAt: '' } };
  },
};

let jobs = [...JOBS];

export const transfers: typeof real.transfers = {
  jobs: async () => [...jobs],
  copy: async () => newId('job'),
  move: async () => newId('job'),
  remove: async () => newId('job'),
  cancel: async () => undefined,
  retry: async () => undefined,
  resume: async () => undefined,
  async clear() {
    jobs = jobs.filter(isJobActive);
  },
};

export const prompts: typeof real.prompts = {
  pending: async () => ({ hostKeys: [], secrets: [], conflicts: [] }),
  respondHostKey: async () => undefined,
  respondSecret: async () => undefined,
  respondConflict: async () => undefined,
};

export const settings: typeof real.settings = {
  get: async () => stored,
  async save(next) {
    stored = { ...next };
    return stored;
  },
  swipeFromScroll: async () => true,
};

export const version: typeof real.version = { info: async () => VERSION };
