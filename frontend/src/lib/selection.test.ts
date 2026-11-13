import { describe, expect, it } from 'vitest';
import {
  EMPTY_SELECTION,
  clickSelect,
  jumpTo,
  moveCursor,
  pruneSelection,
  selectAll,
} from './selection';

const paths = ['a', 'b', 'c', 'd', 'e'];
const plain = { toggle: false, shift: false };
const sorted = (s: ReadonlySet<string>) => [...s].toSorted();

describe('clickSelect', () => {
  it('plain click selects one and sets the anchor', () => {
    const s = clickSelect(EMPTY_SELECTION, paths, 'b', plain);
    expect(sorted(s.selected)).toEqual(['b']);
    expect(s.anchor).toBe('b');
  });

  it('toggle adds and removes', () => {
    let s = clickSelect(EMPTY_SELECTION, paths, 'a', plain);
    s = clickSelect(s, paths, 'c', { toggle: true, shift: false });
    expect(sorted(s.selected)).toEqual(['a', 'c']);
    s = clickSelect(s, paths, 'a', { toggle: true, shift: false });
    expect(sorted(s.selected)).toEqual(['c']);
  });

  it('shift selects the range from the anchor in either direction', () => {
    let s = clickSelect(EMPTY_SELECTION, paths, 'b', plain);
    s = clickSelect(s, paths, 'd', { toggle: false, shift: true });
    expect(sorted(s.selected)).toEqual(['b', 'c', 'd']);
    s = clickSelect(s, paths, 'a', { toggle: false, shift: true });
    expect(sorted(s.selected)).toEqual(['a', 'b']);
    expect(s.anchor).toBe('b');
  });

  it('toggle+shift unions the range with the existing selection', () => {
    let s = clickSelect(EMPTY_SELECTION, paths, 'a', plain);
    s = clickSelect(s, paths, 'c', { toggle: true, shift: false });
    s = clickSelect(s, paths, 'e', { toggle: true, shift: true });
    expect(sorted(s.selected)).toEqual(['a', 'c', 'd', 'e']);
  });

  it('shift without an anchor selects just the clicked row', () => {
    const s = clickSelect(EMPTY_SELECTION, paths, 'c', { toggle: false, shift: true });
    expect(sorted(s.selected)).toEqual(['c']);
  });
});

describe('keyboard movement', () => {
  it('starts at the first row when moving down from nothing', () => {
    expect(moveCursor(EMPTY_SELECTION, paths, 1, false).cursor).toBe('a');
  });
  it('starts at the last row when moving up from nothing', () => {
    expect(moveCursor(EMPTY_SELECTION, paths, -1, false).cursor).toBe('e');
  });
  it('clamps at the ends', () => {
    const s = clickSelect(EMPTY_SELECTION, paths, 'e', plain);
    expect(moveCursor(s, paths, 1, false).cursor).toBe('e');
  });
  it('shift+arrow extends from the anchor', () => {
    let s = clickSelect(EMPTY_SELECTION, paths, 'b', plain);
    s = moveCursor(s, paths, 1, true);
    s = moveCursor(s, paths, 1, true);
    expect(sorted(s.selected)).toEqual(['b', 'c', 'd']);
    s = moveCursor(s, paths, -1, true);
    expect(sorted(s.selected)).toEqual(['b', 'c']);
  });
  it('jumps to edges', () => {
    expect(jumpTo(EMPTY_SELECTION, paths, 'last', false).cursor).toBe('e');
    expect(jumpTo(EMPTY_SELECTION, [], 'first', false)).toBe(EMPTY_SELECTION);
  });
});

describe('selectAll / pruneSelection', () => {
  it('selects every path', () => {
    expect(sorted(selectAll(paths, EMPTY_SELECTION).selected)).toEqual(paths);
  });
  it('drops vanished paths and keeps identity when unchanged', () => {
    const s = selectAll(paths, EMPTY_SELECTION);
    expect(pruneSelection(s, paths)).toBe(s);
    const pruned = pruneSelection(clickSelect(s, paths, 'a', plain), ['b', 'c']);
    expect(pruned.selected.size).toBe(0);
    expect(pruned.anchor).toBeNull();
  });
});
