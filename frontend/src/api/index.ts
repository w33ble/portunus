import type { CancellablePromise } from '@wailsio/runtime';
import {
  ConnectionService,
  FileService,
  PromptService,
  SettingsService,
  TransferService,
} from '../../bindings/github.com/w33ble/portunus/internal/services';
import type { ConflictAnswer } from '../../bindings/github.com/w33ble/portunus/internal/prompt';
import { Policy, TargetKind } from './enums';
import { toAppError } from './errors';
import type {
  Connection,
  Entry,
  Host,
  JobView,
  Pending,
  S3Profile,
  Settings,
  Target,
} from './types';

export { AppError, isCanceled, toAppError } from './errors';

/** Rejects with a typed AppError instead of the raw Wails error. */
function call<T>(promise: PromiseLike<T>): Promise<T> {
  return Promise.resolve(promise).catch((e: unknown) => {
    throw toAppError(e);
  });
}

export interface Listing {
  /** Resolves with the total entry count once List returns. */
  total: Promise<number>;
  cancel(): void;
}

export const files = {
  list(connId: string, dir: string, requestId: string): Listing {
    const promise: CancellablePromise<number> = FileService.List(connId, dir, requestId);
    return {
      total: call(promise),
      cancel: () => {
        void promise.cancel();
      },
    };
  },
  mkdir: (connId: string, dir: string, name: string): Promise<Entry> =>
    call(FileService.Mkdir(connId, dir, name)),
  rename: (connId: string, path: string, newName: string): Promise<Entry> =>
    call(FileService.Rename(connId, path, newName)),
};

export const connections = {
  list: async (): Promise<Connection[]> => (await call(ConnectionService.Connections())) ?? [],
  connect: (target: Target): Promise<Connection> => call(ConnectionService.Connect(target)),
  sshHosts: async (): Promise<Host[]> => (await call(ConnectionService.SSHHosts())) ?? [],
  sshAvailable: (): Promise<string> => call(ConnectionService.SSHAvailable()),
  secretsSessionOnly: (): Promise<boolean> => call(ConnectionService.SecretsSessionOnly()),
  s3Profiles: async (): Promise<S3Profile[]> => (await call(ConnectionService.S3Profiles())) ?? [],
  saveS3Profile: (profile: S3Profile, secret: string): Promise<S3Profile> =>
    call(ConnectionService.SaveS3Profile(profile, secret)),
  deleteS3Profile: (id: string): Promise<void> => call(ConnectionService.DeleteS3Profile(id)),
};

/** Connect targets, built with the generated TargetKind values. */
export const targets = {
  local: (): Target => ({ kind: TargetKind.TargetLocal }),
  ssh: (alias: string): Target => ({ kind: TargetKind.TargetSSH, alias }),
  quick: (user: string, host: string, port: number): Target => ({
    kind: TargetKind.TargetSSH,
    host,
    ...(user ? { user } : {}),
    ...(port ? { port } : {}),
  }),
  s3: (profileId: string): Target => ({ kind: TargetKind.TargetS3, profileId }),
};

export const transfers = {
  jobs: async (): Promise<JobView[]> => (await call(TransferService.Jobs())) ?? [],
  copy: (req: {
    srcConn: string;
    sources: string[];
    dstConn: string;
    dstDir: string;
  }): Promise<string> => call(TransferService.Copy({ ...req, policy: Policy.PolicyAsk })),
  remove: (conn: string, paths: string[]): Promise<string> =>
    call(TransferService.Delete({ conn, paths })),
  cancel: (jobId: string): Promise<void> => call(TransferService.Cancel(jobId)),
  retry: (jobId: string): Promise<void> => call(TransferService.Retry(jobId)),
  clear: (): Promise<void> => call(TransferService.Clear()),
};

export const prompts = {
  pending: (): Promise<Pending> => call(PromptService.Pending()),
  respondHostKey: (id: string, accept: boolean): Promise<void> =>
    call(PromptService.RespondHostKey(id, { accept })),
  respondSecret: (id: string, value: string, cancel: boolean): Promise<void> =>
    call(PromptService.RespondSecret(id, { value, cancel })),
  respondConflict: (id: string, answer: ConflictAnswer): Promise<void> =>
    call(PromptService.RespondConflict(id, answer)),
};

export const settings = {
  get: (): Promise<Settings> => call(SettingsService.Get()),
  save: (next: Settings): Promise<Settings> => call(SettingsService.Save(next)),
};
