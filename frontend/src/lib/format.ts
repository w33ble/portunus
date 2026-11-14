const UNITS = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'] as const;

export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 0) return '—';
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < UNITS.length - 1) {
    value /= 1024;
    unit++;
  }
  const digits = unit === 0 || value >= 100 ? 0 : 1;
  return `${value.toFixed(digits)} ${UNITS[unit]}`;
}

export function formatSpeed(bytesPerSec: number): string {
  if (!Number.isFinite(bytesPerSec) || bytesPerSec <= 0) return '—';
  return `${formatBytes(bytesPerSec)}/s`;
}

/** Returns null while the engine has no estimate (etaSeconds < 0). */
export function formatEta(seconds: number): string | null {
  if (!Number.isFinite(seconds) || seconds < 0) return null;
  const s = Math.round(seconds);
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ${s % 60}s`;
  const h = Math.floor(m / 60);
  return `${h}h ${m % 60}m`;
}

export interface DateFormatOptions {
  now?: Date;
  locale?: string;
  timeZone?: string;
}

function yearOf(date: Date, timeZone: string | undefined): number {
  return Number(new Intl.DateTimeFormat('en-US', { year: 'numeric', timeZone }).format(date));
}

/** Go's zero time.Time and S3 prefixes come through as year 1. */
export function formatModified(iso: string, opts: DateFormatOptions = {}): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime()) || date.getUTCFullYear() <= 1) return '—';

  const { now = new Date(), locale, timeZone } = opts;
  const sameYear = yearOf(date, timeZone) === yearOf(now, timeZone);

  return new Intl.DateTimeFormat(locale, {
    timeZone,
    year: sameYear ? undefined : 'numeric',
    month: 'short',
    day: 'numeric',
    hour: sameYear ? '2-digit' : undefined,
    minute: sameYear ? '2-digit' : undefined,
  }).format(date);
}

export function formatDateTime(iso: string, opts: Omit<DateFormatOptions, 'now'> = {}): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime()) || date.getUTCFullYear() <= 1) return '—';
  return new Intl.DateTimeFormat(opts.locale, {
    timeZone: opts.timeZone,
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(date);
}

/** `ls -l` style permissions ("drwxr-xr-x") from a Go fs.FileMode; "—" where the backend has none (S3). */
export function formatMode(mode: number, isDir: boolean, isLink: boolean): string {
  const perm = mode & 0o777;
  if (perm === 0) return '—';
  const type = isLink ? 'l' : isDir ? 'd' : '-';
  const bits = [...'rwxrwxrwx'].map((c, i) => (perm & (1 << (8 - i)) ? c : '-'));
  return type + bits.join('');
}
