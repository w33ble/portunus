import { vi } from 'vitest';
import { Kind, TargetKind } from '../api/enums';
import type { Connection, Entry } from '../api/types';

export function entry(name: string, isDir = false, dir = '/home'): Entry {
  return {
    name,
    path: `${dir}/${name}`,
    size: 10,
    modTime: '2026-01-01T00:00:00Z',
    mode: 0,
    isDir,
  };
}

export function localConnection(): Connection {
  return {
    id: 'local',
    kind: Kind.KindLocal,
    label: 'Local',
    home: '/home',
    target: { kind: TargetKind.TargetLocal },
  };
}

export interface Deferred<T> {
  promise: Promise<T>;
  resolve: (value: T) => void;
  reject: (reason: unknown) => void;
}

export function deferred<T>(): Deferred<T> {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

/** A binding rejection shaped like Wails': an Error carrying the marshaled `cause`. */
export function bindingError(kind: string, message: string): Error {
  const error = new Error(message);
  error.cause = { kind, message };
  return error;
}

/** Factory for vi.mock('../api'): only the wire boundary is faked. */
export function createApiMock() {
  return {
    files: { list: vi.fn(), mkdir: vi.fn(), rename: vi.fn(), summarize: vi.fn() },
    preview: { load: vi.fn() },
    connections: {
      list: vi.fn(),
      connect: vi.fn(),
      sshHosts: vi.fn(),
      sshAvailable: vi.fn(),
      secretsSessionOnly: vi.fn(),
      s3Profiles: vi.fn(),
      saveS3Profile: vi.fn(),
      deleteS3Profile: vi.fn(),
    },
    targets: {
      local: () => ({ kind: 'local' }),
      ssh: (alias: string) => ({ kind: 'ssh', alias }),
      quick: (user: string, host: string, port: number) => ({ kind: 'ssh', user, host, port }),
      s3: (profileId: string) => ({ kind: 's3', profileId }),
    },
    transfers: {
      jobs: vi.fn(),
      copy: vi.fn(),
      move: vi.fn(),
      remove: vi.fn(),
      cancel: vi.fn(),
      retry: vi.fn(),
      resume: vi.fn(),
      clear: vi.fn(),
    },
    prompts: {
      pending: vi.fn(),
      respondHostKey: vi.fn(),
      respondSecret: vi.fn(),
      respondConflict: vi.fn(),
    },
    settings: { get: vi.fn(), save: vi.fn(), swipeFromScroll: vi.fn() },
    version: { info: vi.fn(() => new Promise(() => {})) },
    ConflictAction: {
      ConflictOverwrite: 'overwrite',
      ConflictSkip: 'skip',
      ConflictCancel: 'cancel',
    },
  };
}
