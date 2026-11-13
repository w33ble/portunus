import { describe, expect, it } from 'vitest';
import type { Entry } from '../api/types';
import { toggleSort, visibleEntries } from './sort';

function entry(name: string, over: Partial<Entry> = {}): Entry {
  return {
    name,
    path: `/${name}`,
    size: 0,
    modTime: '2026-01-01T00:00:00Z',
    mode: 0,
    isDir: false,
    ...over,
  };
}

const names = (es: Entry[]) => es.map((e) => e.name);

describe('visibleEntries', () => {
  const entries = [
    entry('b.txt', { size: 5 }),
    entry('a.txt', { size: 50 }),
    entry('zdir', { isDir: true }),
    entry('Adir', { isDir: true }),
    entry('.hidden'),
    entry('file10'),
    entry('file2'),
  ];

  it('puts directories first and sorts names naturally', () => {
    const out = visibleEntries(entries, true, { key: 'name', dir: 'asc' });
    expect(names(out)).toEqual(['Adir', 'zdir', '.hidden', 'a.txt', 'b.txt', 'file2', 'file10']);
  });

  it('keeps directories first when descending', () => {
    const out = visibleEntries(entries, false, { key: 'name', dir: 'desc' });
    expect(names(out)).toEqual(['zdir', 'Adir', 'file10', 'file2', 'b.txt', 'a.txt']);
  });

  it('filters hidden entries unless asked', () => {
    expect(names(visibleEntries(entries, false, { key: 'name', dir: 'asc' }))).not.toContain(
      '.hidden'
    );
    expect(names(visibleEntries(entries, true, { key: 'name', dir: 'asc' }))).toContain('.hidden');
  });

  it('sorts by size with name tiebreak', () => {
    const out = visibleEntries(entries, false, { key: 'size', dir: 'asc' });
    expect(names(out).slice(2)).toEqual(['file2', 'file10', 'b.txt', 'a.txt']);
  });

  it('sorts by modification time', () => {
    const files = [
      entry('old', { modTime: '2020-01-01T00:00:00Z' }),
      entry('new', { modTime: '2026-01-01T00:00:00Z' }),
    ];
    expect(names(visibleEntries(files, true, { key: 'modTime', dir: 'desc' }))).toEqual([
      'new',
      'old',
    ]);
  });

  it('does not mutate its input', () => {
    const copy = [...entries];
    visibleEntries(entries, true, { key: 'size', dir: 'desc' });
    expect(entries).toEqual(copy);
  });
});

describe('toggleSort', () => {
  it('flips direction on the active column', () => {
    expect(toggleSort({ key: 'name', dir: 'asc' }, 'name')).toEqual({ key: 'name', dir: 'desc' });
  });
  it('switches column ascending, dates newest first', () => {
    expect(toggleSort({ key: 'name', dir: 'desc' }, 'size')).toEqual({ key: 'size', dir: 'asc' });
    expect(toggleSort({ key: 'name', dir: 'asc' }, 'modTime')).toEqual({
      key: 'modTime',
      dir: 'desc',
    });
  });
});
