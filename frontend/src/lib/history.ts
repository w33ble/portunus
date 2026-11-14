import type { Selection } from './selection';

/** A folder in history, with the selection it had when the pane left it. */
export interface Place {
  readonly path: string;
  readonly selection: Selection;
}

/** Per-pane back/forward stacks of places, newest last. */
export interface History {
  readonly back: readonly Place[];
  readonly forward: readonly Place[];
}

export const EMPTY_HISTORY: History = { back: [], forward: [] };

/** Oldest entries are dropped beyond this many per stack. */
export const HISTORY_LIMIT = 50;

const capped = (stack: readonly Place[]) =>
  stack.length > HISTORY_LIMIT ? stack.slice(stack.length - HISTORY_LIMIT) : stack;

/** Records leaving `from` for `to`. Staying put changes nothing; going somewhere new drops forward. */
export function visit(history: History, from: Place, to: string): History {
  if (from.path === to) return history;
  return { back: capped([...history.back, from]), forward: [] };
}

export interface Step {
  history: History;
  place: Place;
}

/** Steps back from `current`, or null when there's nowhere to go. */
export function back(history: History, current: Place): Step | null {
  const place = history.back.at(-1);
  if (place === undefined) return null;
  return {
    place,
    history: { back: history.back.slice(0, -1), forward: capped([...history.forward, current]) },
  };
}

/** Steps forward from `current`, or null when there's nowhere to go. */
export function forward(history: History, current: Place): Step | null {
  const place = history.forward.at(-1);
  if (place === undefined) return null;
  return {
    place,
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

/** Back/forward for the extra mouse buttons: MouseEvent.button 3 and 4. */
export function historyButton(button: number): 'back' | 'forward' | null {
  return button === 3 ? 'back' : button === 4 ? 'forward' : null;
}
