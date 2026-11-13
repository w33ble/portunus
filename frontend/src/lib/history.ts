/** Per-pane back/forward stacks of directory paths, newest last. */
export interface History {
  readonly back: readonly string[];
  readonly forward: readonly string[];
}

export const EMPTY_HISTORY: History = { back: [], forward: [] };

/** Oldest entries are dropped beyond this many per stack. */
export const HISTORY_LIMIT = 50;

const capped = (stack: readonly string[]) =>
  stack.length > HISTORY_LIMIT ? stack.slice(stack.length - HISTORY_LIMIT) : stack;

/** Records leaving `from` for `to`. Staying put changes nothing; going somewhere new drops forward. */
export function visit(history: History, from: string, to: string): History {
  if (from === to) return history;
  return { back: capped([...history.back, from]), forward: [] };
}

export interface Step {
  history: History;
  path: string;
}

/** Steps back from `current`, or null when there's nowhere to go. */
export function back(history: History, current: string): Step | null {
  const path = history.back.at(-1);
  if (path === undefined) return null;
  return {
    path,
    history: { back: history.back.slice(0, -1), forward: capped([...history.forward, current]) },
  };
}

/** Steps forward from `current`, or null when there's nowhere to go. */
export function forward(history: History, current: string): Step | null {
  const path = history.forward.at(-1);
  if (path === undefined) return null;
  return {
    path,
    history: { back: capped([...history.back, current]), forward: history.forward.slice(0, -1) },
  };
}

interface NavKeyEvent {
  key: string;
  metaKey: boolean;
  ctrlKey: boolean;
  altKey: boolean;
  shiftKey: boolean;
}

/** Back/forward shortcut for a key event: ⌘[ / ⌘] on macOS, Alt+←/→ elsewhere. */
export function historyKey(e: NavKeyEvent, mac: boolean): 'back' | 'forward' | null {
  if (e.shiftKey) return null;
  if (mac) {
    if (!e.metaKey || e.ctrlKey || e.altKey) return null;
    return e.key === '[' ? 'back' : e.key === ']' ? 'forward' : null;
  }
  if (!e.altKey || e.ctrlKey || e.metaKey) return null;
  return e.key === 'ArrowLeft' ? 'back' : e.key === 'ArrowRight' ? 'forward' : null;
}

/** Shortcut hint for tooltips, matching historyKey. */
export function historyShortcut(direction: 'back' | 'forward', mac: boolean): string {
  if (mac) return direction === 'back' ? '⌘[' : '⌘]';
  return direction === 'back' ? 'Alt+←' : 'Alt+→';
}
