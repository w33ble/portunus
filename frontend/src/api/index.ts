import {
  ConnectionService,
  FileService,
  PromptService,
  SettingsService,
  TransferService,
  VersionService,
} from '../../bindings/github.com/w33ble/portunus/internal/services';
import type { ConflictAnswer } from '../../bindings/github.com/w33ble/portunus/internal/prompt';
import { Policy, TargetKind } from './enums';
import { toAppError } from './errors';
import { type ListBackend, type Listing, pullListing } from './listing';
import { loadPreview } from './preview';
import type {
  Connection,
  DirSummary,
  Entry,
  Host,
  JobView,
  Pending,
  S3Profile,
  Settings,
  VersionInfo,
  Target,
} from './types';

export { AppError, isCanceled, toAppError } from './errors';

/** Rejects with a typed AppError instead of the raw Wails error. */
function call<T>(promise: PromiseLike<T>): Promise<T> {
  return Promise.resolve(promise).catch((e: unknown) => {
    throw toAppError(e);
  });
}

export type { Listing } from './listing';
export type { NoPreviewReason, PreviewContent } from './preview';

/** Like call(), and aborting `signal` cancels the in-flight request (Wails' CancellablePromise). */
function callWithSignal<T>(
  promise: PromiseLike<T> & { cancel(): unknown },
  signal: AbortSignal
): Promise<T> {
  const cancel = () => void Promise.resolve(promise.cancel()).catch(() => undefined);
  signal.addEventListener('abort', cancel, { once: true });
  return call(promise).finally(() => signal.removeEventListener('abort', cancel));
}

const listBackend: ListBackend = {
  start: (connId, dir) => FileService.List(connId, dir),
  next: (cursor) => FileService.ListNext(cursor),
  close: (cursor) => FileService.ListClose(cursor),
};

export const files = {
  /** Pulls the listing page by page, calling onPage with each as it arrives. */
  list: (connId: string, dir: string, onPage: (entries: readonly Entry[]) => void): Listing =>
    pullListing(listBackend, connId, dir, onPage),
  mkdir: (connId: string, dir: string, name: string): Promise<Entry> =>
    call(FileService.Mkdir(connId, dir, name)),
  rename: (connId: string, path: string, newName: string): Promise<Entry> =>
    call(FileService.Rename(connId, path, newName)),
  /** Counts a directory's immediate entries for the preview. */
  summarize: (connId: string, dir: string, signal: AbortSignal): Promise<DirSummary> =>
    callWithSignal(FileService.Summarize(connId, dir), signal),
};

export const preview = {
  load: loadPreview,
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
  move: (req: {
    srcConn: string;
    sources: string[];
    dstConn: string;
    dstDir: string;
  }): Promise<string> => call(TransferService.Move({ ...req, policy: Policy.PolicyAsk })),
  remove: (conn: string, paths: string[]): Promise<string> =>
    call(TransferService.Delete({ conn, paths })),
  cancel: (jobId: string): Promise<void> => call(TransferService.Cancel(jobId)),
  retry: (jobId: string): Promise<void> => call(TransferService.Retry(jobId)),
  resume: (jobId: string): Promise<void> => call(TransferService.Resume(jobId)),
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
  /** Whether the OS treats two-finger sideways scrolls as back/forward swipes. */
  swipeFromScroll: (): Promise<boolean> => call(SettingsService.SwipeFromScroll()),
};

export const version = {
  info: (): Promise<VersionInfo> => call(VersionService.Info()),
};
