import { FileMode } from '../bindings/io/fs/models';
import type { Entry } from '../src/api/types';

export const MIN = 60_000;
export const HOUR = 60 * MIN;
export const DAY = 24 * HOUR;
const KB = 1024;
const MB = 1024 * KB;
const GB = 1024 * MB;

/** An ISO timestamp `ms` before now, so every listing looks recently touched. */
export const ago = (ms: number): string => new Date(Date.now() - ms).toISOString();

/** What Go sends for S3 buckets and prefixes, which have no modification time. */
const ZERO_TIME = '0001-01-01T00:00:00Z';

interface Item {
  name: string;
  isDir: boolean;
  size: number;
  modTime: string;
  perm: number;
  link?: string;
}

const dir = (name: string, age: number, perm = 0o755): Item => ({
  name,
  isDir: true,
  size: 0,
  modTime: ago(age),
  perm,
});

const file = (name: string, size: number, age: number, perm = 0o644): Item => ({
  name,
  isDir: false,
  size: Math.round(size),
  modTime: ago(age),
  perm,
});

const link = (name: string, to: string, age: number): Item => ({
  name,
  isDir: true,
  size: 0,
  modTime: ago(age),
  perm: 0o777,
  link: to,
});

/** S3 has no permissions and no times. */
const prefix = (name: string): Item => ({
  name,
  isDir: true,
  size: 0,
  modTime: ZERO_TIME,
  perm: 0,
});

const object = (name: string, size: number, age: number): Item => ({
  name,
  isDir: false,
  size: Math.round(size),
  modTime: ago(age),
  perm: 0,
});

function entries(parent: string, items: readonly Item[]): Entry[] {
  const base = parent.endsWith('/') ? parent : `${parent}/`;
  return items.map((item) => {
    const kind = item.link ? FileMode.ModeSymlink : item.isDir ? FileMode.ModeDir : 0;
    const entry: Entry = {
      name: item.name,
      path: base + item.name,
      size: item.size,
      modTime: item.modTime,
      mode: kind | item.perm,
      isDir: item.isDir,
    };
    return item.link ? { ...entry, link: item.link } : entry;
  });
}

/** Directory listings by absolute path; parents of listed paths are filled in on demand. */
export type Tree = Map<string, Entry[]>;

const tree = (listings: Record<string, readonly Item[]>): Tree =>
  new Map(Object.entries(listings).map(([parent, items]) => [parent, entries(parent, items)]));

export const LOCAL_TREE = tree({
  '/Users/joe': [
    dir('Desktop', 2 * DAY),
    dir('Documents', 5 * HOUR),
    dir('Downloads', 3 * HOUR),
    dir('Movies', 41 * DAY),
    dir('Music', 90 * DAY),
    dir('Pictures', 6 * DAY),
    dir('Projects', 30 * MIN),
    file('.gitconfig', 312, 120 * DAY),
    file('.zshrc', 2 * KB, 9 * DAY),
  ],
  '/Users/joe/Projects': [
    dir('api-gateway', 3 * DAY),
    dir('dotfiles', 12 * DAY),
    dir('notes', 1 * DAY),
    dir('website', 30 * MIN),
  ],
  '/Users/joe/Projects/website': [
    dir('dist', 2 * HOUR),
    dir('node_modules', 4 * DAY),
    dir('public', 6 * DAY),
    dir('src', 3 * HOUR),
    file('.gitignore', 96, 52 * DAY),
    file('.env.example', 214, 40 * DAY),
    file('.prettierrc', 88, 52 * DAY),
    file('CHANGELOG.md', 7.9 * KB, 2 * DAY),
    file('Dockerfile', 612, 33 * DAY),
    file('LICENSE', 1.1 * KB, 200 * DAY),
    file('deploy.sh', 1.4 * KB, 8 * DAY, 0o755),
    file('index.html', 1.1 * KB, 6 * DAY),
    file('package.json', 1.8 * KB, 4 * DAY),
    file('pnpm-lock.yaml', 212 * KB, 4 * DAY),
    file('README.md', 3.6 * KB, 19 * DAY),
    file('tsconfig.json', 604, 52 * DAY),
    file('vite.config.ts', 889, 11 * DAY),
  ],
  '/Users/joe/Downloads': [
    file('access.log.1', 31.2 * MB, 3 * HOUR),
    file('access.log.2.gz', 4.4 * MB, 3 * HOUR),
    file('error.log.1', 612 * KB, 3 * HOUR),
    file('invoice-0617.pdf', 184 * KB, 1 * DAY),
  ],
});

const RELEASE = '/var/www/site/releases/20260617-2';

export const WEB_PROD_TREE = tree({
  '/home/deploy': [
    dir('bin', 90 * DAY),
    dir('logs', 1 * DAY),
    file('.bash_history', 14 * KB, 2 * HOUR),
    file('.profile', 807, 200 * DAY),
  ],
  '/var/www/site': [
    dir('assets', 1 * DAY),
    dir('releases', 1 * DAY),
    link('current', RELEASE, 1 * DAY),
    file('index.html', 6.3 * KB, 1 * DAY),
    file('robots.txt', 142, 64 * DAY),
    file('404.html', 2.4 * KB, 12 * DAY),
    file('favicon.ico', 15 * KB, 64 * DAY),
    file('feed.xml', 9.2 * KB, 1 * DAY),
    file('manifest.webmanifest', 480, 64 * DAY),
    file('sitemap.xml', 18.7 * KB, 1 * DAY),
  ],
  '/var/www/site/assets': [
    dir('fonts', 64 * DAY),
    dir('img', 8 * DAY),
    file('app.3f9a1c.css', 38.5 * KB, 1 * DAY),
    file('app.3f9a1c.js', 214 * KB, 1 * DAY),
    file('vendor.b71e04.js', 142 * KB, 1 * DAY),
  ],
  '/var/www/site/releases': [
    dir('20260611-1', 7 * DAY),
    dir('20260614-1', 4 * DAY),
    dir('20260617-1', 1.2 * DAY),
    dir('20260617-2', 1 * DAY),
  ],
  [RELEASE]: [
    dir('assets', 1 * DAY),
    file('index.html', 6.3 * KB, 1 * DAY),
    file('sitemap.xml', 18.7 * KB, 1 * DAY),
  ],
  '/var/log/nginx': [
    file('access.log', 48.3 * MB, 4 * MIN, 0o640),
    file('access.log.1', 31.2 * MB, 1 * DAY, 0o640),
    file('access.log.2.gz', 4.4 * MB, 2 * DAY, 0o640),
    file('error.log', 612 * KB, 2 * HOUR, 0o640),
    file('error.log.1', 1.9 * MB, 1 * DAY, 0o640),
  ],
});

/** A home directory with a few typical entries, for hosts the screenshots don't browse. */
const home = (path: string, ...items: Item[]): Tree => tree({ [path]: items });

export const STAGING_TREE = home(
  '/home/deploy',
  dir('site', 2 * HOUR),
  file('.profile', 807, 160 * DAY)
);

export const NAS_TREE = home(
  '/volume1/homes/joe',
  dir('backups', 3 * DAY),
  dir('music', 120 * DAY),
  dir('photos', 9 * DAY),
  file('notes.txt', 2.2 * KB, 33 * DAY)
);

export const BASTION_TREE = home('/home/joe', file('.profile', 807, 300 * DAY));

export const BUILD_TREE = home(
  '/home/ci',
  dir('workspace', 40 * MIN),
  file('runner.log', 8.8 * MB, 40 * MIN)
);

export const PI_TREE = home(
  '/home/pi',
  dir('photoprism', 11 * DAY),
  file('backup.sh', 960, 77 * DAY, 0o755),
  file('hass-config.tar.gz', 86 * MB, 6 * DAY)
);

export const BACKUPS_TREE = tree({
  '/': [prefix('laptop-photos'), prefix('site-backups')],
  '/site-backups': [prefix('daily'), prefix('weekly')],
  '/site-backups/daily': [
    object('site-2026-06-15.tar.zst', 612 * MB, 3 * DAY),
    object('site-2026-06-16.tar.zst', 618 * MB, 2 * DAY),
    object('site-2026-06-17.tar.zst', 621 * MB, 1 * DAY),
  ],
  '/laptop-photos': [prefix('2025'), prefix('2026')],
  '/laptop-photos/2026': [
    object('IMG_2041.heic', 3.1 * MB, 9 * DAY),
    object('IMG_2042.heic', 2.8 * MB, 9 * DAY),
    object('trip-lisbon.mov', 1.4 * GB, 9 * DAY),
  ],
});

export const MEDIA_TREE = tree({
  '/': [prefix('cdn-assets'), prefix('video-originals')],
  '/cdn-assets': [prefix('css'), prefix('fonts'), prefix('img')],
  '/cdn-assets/img': [
    object('hero.avif', 184 * KB, 12 * DAY),
    object('hero.webp', 241 * KB, 12 * DAY),
    object('logo.svg', 4.2 * KB, 40 * DAY),
  ],
});
