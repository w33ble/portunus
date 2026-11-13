export interface Selection {
  selected: ReadonlySet<string>;
  /** Fixed end of a shift-range. */
  anchor: string | null;
  /** Row with keyboard focus; moves with arrows. */
  cursor: string | null;
}

export const EMPTY_SELECTION: Selection = { selected: new Set(), anchor: null, cursor: null };

export interface Modifiers {
  /** Cmd on macOS, Ctrl elsewhere. */
  toggle: boolean;
  shift: boolean;
}

function rangeBetween(paths: readonly string[], a: string | null, b: string): string[] {
  const to = paths.indexOf(b);
  const from = a === null ? to : paths.indexOf(a);
  if (to < 0) return [];
  if (from < 0) return [b];
  return paths.slice(Math.min(from, to), Math.max(from, to) + 1);
}

/** Row click with modifiers; `paths` is the visible order. */
export function clickSelect(
  sel: Selection,
  paths: readonly string[],
  path: string,
  mods: Modifiers
): Selection {
  if (mods.shift) {
    const range = rangeBetween(paths, sel.anchor ?? sel.cursor, path);
    const selected = mods.toggle ? new Set([...sel.selected, ...range]) : new Set(range);
    return { selected, anchor: sel.anchor ?? sel.cursor ?? path, cursor: path };
  }
  if (mods.toggle) {
    const selected = new Set(sel.selected);
    if (!selected.delete(path)) selected.add(path);
    return { selected, anchor: path, cursor: path };
  }
  return { selected: new Set([path]), anchor: path, cursor: path };
}

export function selectAll(paths: readonly string[], sel: Selection): Selection {
  return {
    selected: new Set(paths),
    anchor: paths[0] ?? null,
    cursor: sel.cursor ?? paths[0] ?? null,
  };
}

/** Arrow-key movement by `delta` rows (clamped). `extend` grows the range from the anchor. */
export function moveCursor(
  sel: Selection,
  paths: readonly string[],
  delta: number,
  extend: boolean
): Selection {
  if (paths.length === 0) return EMPTY_SELECTION;
  const current = sel.cursor === null ? -1 : paths.indexOf(sel.cursor);
  const base = current < 0 ? (delta > 0 ? -1 : paths.length) : current;
  const next = paths[Math.min(Math.max(base + delta, 0), paths.length - 1)];
  if (next === undefined) return sel;
  return clickSelect(sel, paths, next, { toggle: false, shift: extend });
}

export function jumpTo(
  sel: Selection,
  paths: readonly string[],
  edge: 'first' | 'last',
  extend: boolean
): Selection {
  const target = edge === 'first' ? paths[0] : paths[paths.length - 1];
  if (target === undefined) return EMPTY_SELECTION;
  return clickSelect(sel, paths, target, { toggle: false, shift: extend });
}

/** Drops paths that are no longer listed. Returns the same object when nothing changed. */
export function pruneSelection(sel: Selection, paths: readonly string[]): Selection {
  const present = new Set(paths);
  const stale = [...sel.selected].some((p) => !present.has(p));
  const anchorGone = sel.anchor !== null && !present.has(sel.anchor);
  const cursorGone = sel.cursor !== null && !present.has(sel.cursor);
  if (!stale && !anchorGone && !cursorGone) return sel;
  return {
    selected: new Set([...sel.selected].filter((p) => present.has(p))),
    anchor: anchorGone ? null : sel.anchor,
    cursor: cursorGone ? null : sel.cursor,
  };
}

export function selectOnly(path: string): Selection {
  return { selected: new Set([path]), anchor: path, cursor: path };
}
