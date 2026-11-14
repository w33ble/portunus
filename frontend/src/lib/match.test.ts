import { describe, expect, it } from 'vitest';
import type { Entry } from '../api/types';
import {
  EMPTY_TYPE_AHEAD,
  TYPE_AHEAD_RESET_MS,
  filterEntries,
  matchName,
  typeAheadIndex,
  typeKey,
} from './match';

function entry(name: string): Entry {
  return {
    name,
    path: `/d/${name}`,
    size: 0,
    modTime: '2026-01-01T00:00:00Z',
    mode: 0,
    isDir: false,
  };
}

const names = (entries: readonly Entry[]) => entries.map((e) => e.name);

describe('filterEntries', () => {
  const entries = ['thread.go', 'notes.txt', 'read-file-name.ts', 'README.md'].map(entry);

  it('returns the list untouched for a blank query', () => {
    expect(filterEntries(entries, '')).toBe(entries);
    expect(filterEntries(entries, '   ')).toBe(entries);
  });

  it('drops non-matches and ranks prefix matches above mid-word ones, ignoring case', () => {
    expect(names(filterEntries(entries, 'READ'))).toEqual([
      'README.md',
      'read-file-name.ts',
      'thread.go',
    ]);
  });

  it('matches scattered characters, such as word initials', () => {
    expect(names(filterEntries(entries, 'rfn'))).toEqual(['read-file-name.ts']);
  });

  it('keeps the list order among equally good matches', () => {
    const photos = ['photo_2.jpg', 'photo_10.jpg', 'photo_11.jpg', 'photo_19.jpg', 'photo_14.jpg'];
    expect(names(filterEntries(photos.map(entry), 'pho1'))).toEqual([
      'photo_10.jpg',
      'photo_11.jpg',
      'photo_19.jpg',
      'photo_14.jpg',
    ]);
  });

  it('matches decomposed (NFD) names against composed input and plain letters', () => {
    const cafe = [entry('Cafe\u0301 menu.pdf'), entry('other.txt')];
    expect(names(filterEntries(cafe, 'café'))).toEqual(['Cafe\u0301 menu.pdf']);
    expect(names(filterEntries(cafe, 'cafe'))).toEqual(['Cafe\u0301 menu.pdf']);
  });
});

describe('matchName', () => {
  it('reports matched positions in the composed name, so accents highlight with their letter', () => {
    expect(matchName('Cafe\u0301 menu.pdf', 'menu')?.indexes).toEqual([5, 6, 7, 8]);
  });
});

describe('typeAheadIndex', () => {
  const list = ['Applications', 'apple.txt', 'banana', 'Bandit', 'b2', 'cherry'];

  it('lands on the first name starting with the typed prefix, ignoring case', () => {
    expect(typeAheadIndex(list, 'b', -1)).toBe(2);
    expect(typeAheadIndex(list, 'BAND', -1)).toBe(3);
    expect(typeAheadIndex(list, 'appl', 4)).toBe(0);
  });

  it('stays put when nothing starts with the prefix', () => {
    expect(typeAheadIndex(list, 'zz', 1)).toBe(-1);
    expect(typeAheadIndex(list, 'bx', 1)).toBe(-1);
  });

  it('steps through names on one letter when it is repeated, wrapping at the end', () => {
    expect(typeAheadIndex(list, 'bb', 2)).toBe(3);
    expect(typeAheadIndex(list, 'bbb', 3)).toBe(4);
    expect(typeAheadIndex(list, 'bbbb', 4)).toBe(2);
  });

  it('matches decomposed names against composed input', () => {
    expect(typeAheadIndex(['zeta', 'e\u0301clair'], 'é', -1)).toBe(1);
  });
});

describe('typeKey', () => {
  it('extends the prefix while typing and starts over after a pause', () => {
    let state = typeKey(EMPTY_TYPE_AHEAD, 'r', 1000);
    state = typeKey(state, 'e', 1500);
    expect(state.text).toBe('re');
    state = typeKey(state, 'a', 1500 + TYPE_AHEAD_RESET_MS);
    expect(state.text).toBe('rea');
    state = typeKey(state, 'n', 1501 + 2 * TYPE_AHEAD_RESET_MS);
    expect(state.text).toBe('n');
  });
});
