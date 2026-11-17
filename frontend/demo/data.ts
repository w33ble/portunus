import { Color, JobKind, Kind, State, TargetKind } from '../src/api/enums';
import type {
  Bookmark,
  Connection,
  Host,
  JobView,
  Prefs,
  S3Profile,
  SavedHost,
  Settings,
  SSHHostDetails,
  Target,
  VersionInfo,
} from '../src/api/types';
import {
  BACKUPS_TREE,
  BASTION_TREE,
  BUILD_TREE,
  DAY,
  HOUR,
  LOCAL_TREE,
  MEDIA_TREE,
  MIN,
  NAS_TREE,
  PI_TREE,
  STAGING_TREE,
  type Tree,
  WEB_PROD_TREE,
  ago,
} from './trees';

const MB = 1024 * 1024;
const GB = 1024 * MB;

/** A connection the demo can dial: what `connect` returns and the files behind it. */
export interface Machine {
  connection: Connection;
  tree: Tree;
}

function machine(
  id: string,
  kind: Kind,
  label: string,
  home: string,
  target: Target,
  tree: Tree
): [string, Machine] {
  return [id, { connection: { id, kind, label, home, target }, tree }];
}

const ssh = (alias: string): Target => ({ kind: TargetKind.TargetSSH, alias });
const s3 = (profileId: string): Target => ({ kind: TargetKind.TargetS3, profileId });

export const PI_ID = 'ssh:pi@192.168.1.40';

export const MACHINES: ReadonlyMap<string, Machine> = new Map([
  machine(
    'local',
    Kind.KindLocal,
    'Local',
    '/Users/joe',
    { kind: TargetKind.TargetLocal },
    LOCAL_TREE
  ),
  machine(
    'ssh:web-prod',
    Kind.KindSFTP,
    'web-prod',
    '/home/deploy',
    ssh('web-prod'),
    WEB_PROD_TREE
  ),
  machine('ssh:staging', Kind.KindSFTP, 'staging', '/home/deploy', ssh('staging'), STAGING_TREE),
  machine('ssh:nas', Kind.KindSFTP, 'nas', '/volume1/homes/joe', ssh('nas'), NAS_TREE),
  machine('ssh:bastion', Kind.KindSFTP, 'bastion', '/home/joe', ssh('bastion'), BASTION_TREE),
  machine('ssh:build-01', Kind.KindSFTP, 'build-01', '/home/ci', ssh('build-01'), BUILD_TREE),
  machine(
    PI_ID,
    Kind.KindSFTP,
    'pi@192.168.1.40',
    '/home/pi',
    { kind: TargetKind.TargetSSH, host: '192.168.1.40', user: 'pi' },
    PI_TREE
  ),
  machine('s3:backups', Kind.KindS3, 'Backups', '/', s3('backups'), BACKUPS_TREE),
  machine('s3:media-cdn', Kind.KindS3, 'media-cdn', '/', s3('media-cdn'), MEDIA_TREE),
]);

/** Connections that are already open when the app starts, so transfers can name them. */
export const INITIAL_LIVE: readonly string[] = ['local', 'ssh:web-prod', 'ssh:nas', 's3:backups'];

const CONFIG = '/Users/joe/.ssh/config';

export const SSH_HOSTS: Host[] = [
  { alias: 'web-prod', source: `${CONFIG}:3` },
  { alias: 'staging', source: `${CONFIG}:11` },
  { alias: 'nas', source: `${CONFIG}:17` },
  { alias: 'bastion', source: `${CONFIG}:23` },
  { alias: 'build-01', source: `${CONFIG}:28` },
];

/** What `ssh -G` resolves for each alias. */
export const SSH_DETAILS: Readonly<Record<string, SSHHostDetails>> = {
  'web-prod': { hostName: 'web1.example.com', user: 'deploy', port: 22, proxyJump: 'bastion' },
  staging: { hostName: 'staging.example.com', user: 'deploy', port: 22, proxyJump: 'bastion' },
  nas: { hostName: 'nas.local', user: 'joe', port: 2222, proxyJump: '' },
  bastion: { hostName: 'bastion.example.com', user: 'joe', port: 22, proxyJump: '' },
  'build-01': { hostName: 'build-01.internal', user: 'ci', port: 22, proxyJump: 'bastion' },
};

export const SAVED_HOSTS: SavedHost[] = [
  { id: 'pi', name: 'Pi in the closet', user: 'pi', host: '192.168.1.40', connId: PI_ID },
];

export const S3_PROFILES: S3Profile[] = [
  {
    id: 'backups',
    name: 'Backups',
    accessKeyId: '004a7c1e90b2d3f0000000003',
    region: 'us-west-004',
    endpoint: 'https://s3.us-west-004.backblazeb2.com',
  },
  {
    id: 'media-cdn',
    name: 'media-cdn',
    accessKeyId: '5d1f0c2ab87e4096b3c1e7a2f48d9015',
    region: 'auto',
    endpoint: 'https://8f2c1e5ab97d4036a1b8e2c47d905f13.r2.cloudflarestorage.com',
    pathStyle: true,
  },
];

export const PREFS: Record<string, Prefs> = {
  'ssh:web-prod': { color: Color.ColorRed, favorite: true, openAt: '/var/www/site' },
  'ssh:staging': { color: Color.ColorOrange },
  'ssh:nas': { color: Color.ColorGreen, favorite: true },
  's3:backups': { color: Color.ColorBlue, favorite: true },
  's3:media-cdn': { color: Color.ColorPurple },
};

export const BOOKMARKS: Bookmark[] = [
  {
    id: 'bm-logs',
    connId: 'ssh:web-prod',
    path: '/var/log/nginx',
    name: 'nginx logs',
    pinned: true,
  },
  { id: 'bm-site', connId: 'ssh:web-prod', path: '/var/www/site', name: 'site' },
  { id: 'bm-releases', connId: 'ssh:web-prod', path: '/var/www/site/releases', name: 'releases' },
];

export const SETTINGS: Settings = {
  globalTransfers: 4,
  perConnectionTransfers: 2,
  loadAppleKeychain: true,
  showHidden: true,
  swipeNavigation: true,
  theme: 'system',
};

export const VERSION: VersionInfo = {
  version: 'v0.4.0',
  commit: '8c41d2e',
  date: '2026-06-16T09:12:00Z',
  go: 'go1.27',
};

type JobInit = Pick<JobView, 'id' | 'kind' | 'state' | 'srcConn' | 'dstConn' | 'dstDir' | 'title'> &
  Partial<JobView> & { startedAgo: number };

function job({ startedAgo, ...init }: JobInit): JobView {
  const view: JobView = {
    walking: false,
    totalFiles: 1,
    doneFiles: 0,
    failedFiles: 0,
    skippedFiles: 0,
    totalBytes: 0,
    doneBytes: 0,
    bytesPerSec: 0,
    etaSeconds: -1,
    active: [],
    failed: [],
    createdAt: ago(startedAgo),
    ...init,
  };
  // Sizes are written as fractions of a unit, but the wire carries whole bytes.
  return {
    ...view,
    totalBytes: Math.round(view.totalBytes),
    doneBytes: Math.round(view.doneBytes),
    bytesPerSec: Math.round(view.bytesPerSec),
  };
}

const { StateRunning, StateDone, StateFailed, StateCanceled } = State;

export const JOBS: JobView[] = [
  job({
    id: 'job-6',
    kind: JobKind.KindCopy,
    state: StateRunning,
    srcConn: 'local',
    dstConn: 'ssh:web-prod',
    dstDir: '/var/www/site',
    title: 'dist/ and 1 more',
    totalFiles: 212,
    doneFiles: 127,
    totalBytes: 94.6 * MB,
    doneBytes: 56.8 * MB,
    bytesPerSec: 4.2 * MB,
    etaSeconds: 9,
    startedAgo: 14_000,
  }),
  job({
    id: 'job-5',
    kind: JobKind.KindCopy,
    state: StateRunning,
    srcConn: 'local',
    dstConn: 's3:backups',
    dstDir: '/laptop-photos/2026',
    title: 'Lisbon/',
    totalFiles: 1204,
    doneFiles: 512,
    totalBytes: 3.8 * GB,
    doneBytes: 1.6 * GB,
    bytesPerSec: 11.8 * MB,
    etaSeconds: 187,
    startedAgo: 2 * MIN,
  }),
  job({
    id: 'job-4',
    kind: JobKind.KindCopy,
    state: StateDone,
    srcConn: 'ssh:web-prod',
    dstConn: 'local',
    dstDir: '/Users/joe/Downloads',
    title: 'access.log.1 and 2 more',
    totalFiles: 3,
    doneFiles: 3,
    totalBytes: 36.2 * MB,
    doneBytes: 36.2 * MB,
    startedAgo: 3 * HOUR,
  }),
  job({
    id: 'job-3',
    kind: JobKind.KindCopy,
    state: StateDone,
    srcConn: 'local',
    dstConn: 's3:backups',
    dstDir: '/site-backups/daily',
    title: 'site-2026-06-17.tar.zst',
    totalBytes: 621 * MB,
    doneFiles: 1,
    doneBytes: 621 * MB,
    startedAgo: 1 * DAY,
  }),
  job({
    id: 'job-2',
    kind: JobKind.KindCopy,
    state: StateFailed,
    srcConn: 'local',
    dstConn: 'ssh:nas',
    dstDir: '/volume1/homes/joe/backups',
    title: 'Documents/',
    totalFiles: 38,
    doneFiles: 36,
    failedFiles: 2,
    totalBytes: 412 * MB,
    doneBytes: 398 * MB,
    startedAgo: 1 * DAY + 2 * HOUR,
    failed: [
      {
        id: 'task-1',
        src: '/Users/joe/Documents/taxes-2025.numbers',
        dst: '/volume1/homes/joe/backups/Documents/taxes-2025.numbers',
        size: 9 * MB,
        done: 0,
        state: StateFailed,
        error: 'permission denied',
      },
    ],
  }),
  job({
    id: 'job-1',
    kind: JobKind.KindCopy,
    state: StateCanceled,
    srcConn: 'ssh:web-prod',
    dstConn: 'local',
    dstDir: '/Users/joe/Downloads',
    title: 'access.log',
    totalBytes: 48.3 * MB,
    doneFiles: 0,
    doneBytes: 12.1 * MB,
    startedAgo: 2 * DAY,
  }),
];
