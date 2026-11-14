import { describe, expect, it } from 'vitest';
import {
  formatBytes,
  formatDateTime,
  formatEta,
  formatMode,
  formatModified,
  formatSpeed,
} from './format';

describe('formatBytes', () => {
  it('scales units', () => {
    expect(formatBytes(0)).toBe('0 B');
    expect(formatBytes(1023)).toBe('1023 B');
    expect(formatBytes(1024)).toBe('1.0 KB');
    expect(formatBytes(1536)).toBe('1.5 KB');
    expect(formatBytes(150 * 1024 * 1024)).toBe('150 MB');
    expect(formatBytes(5 * 1024 ** 4)).toBe('5.0 TB');
  });
  it('rejects bad input', () => {
    expect(formatBytes(-1)).toBe('—');
    expect(formatBytes(Number.NaN)).toBe('—');
  });
});

describe('formatSpeed', () => {
  it('appends /s and hides zero', () => {
    expect(formatSpeed(2048)).toBe('2.0 KB/s');
    expect(formatSpeed(0)).toBe('—');
  });
});

describe('formatEta', () => {
  it('formats ranges', () => {
    expect(formatEta(5)).toBe('5s');
    expect(formatEta(75)).toBe('1m 15s');
    expect(formatEta(3725)).toBe('1h 2m');
  });
  it('is null when unknown', () => {
    expect(formatEta(-1)).toBeNull();
  });
});

describe('formatModified', () => {
  const now = new Date('2026-11-12T12:00:00Z');
  const opts = { now, locale: 'en-US', timeZone: 'UTC' };
  it('shows time within the current year', () => {
    expect(formatModified('2026-03-04T15:30:00Z', opts)).toBe('Mar 4, 03:30 PM');
  });
  it('shows the year for older dates', () => {
    expect(formatModified('2024-03-04T15:30:00Z', opts)).toBe('Mar 4, 2024');
  });
  it('dashes zero and invalid times', () => {
    expect(formatModified('0001-01-01T00:00:00Z', opts)).toBe('—');
    expect(formatModified('nope', opts)).toBe('—');
    expect(formatDateTime('0001-01-01T00:00:00Z')).toBe('—');
  });
});

describe('formatMode', () => {
  it('renders permission bits with the entry type', () => {
    expect(formatMode(0o644, false, false)).toBe('-rw-r--r--');
    // Go's fs.ModeDir is bit 31, past JS's int32 sign bit.
    expect(formatMode(2 ** 31 + 0o755, true, false)).toBe('drwxr-xr-x');
    expect(formatMode(0o777, false, true)).toBe('lrwxrwxrwx');
  });
  it('shows a dash where the backend has no permissions', () => {
    expect(formatMode(0, false, false)).toBe('—');
  });
});
