/**
 * Two-finger horizontal swipe detection from wheel events, for trackpads on macOS and
 * Linux. Webviews give no gesture phases, so a gesture is a run of wheel events with no
 * pause longer than GESTURE_GAP_MS; the momentum events after the fingers lift belong to it.
 */

/** Horizontal travel, in CSS pixels, that turns a swipe into back or forward. */
export const SWIPE_THRESHOLD = 150;
/** Travel before a gesture is classed as a horizontal swipe or a vertical scroll. */
export const DECIDE_PX = 12;
/** A pause this long between wheel events ends the gesture, momentum included. */
export const GESTURE_GAP_MS = 150;

const LINE_PX = 16;
const PAGE_PX = 400;

export type SwipeDirection = 'back' | 'forward';

export interface WheelSample {
  deltaX: number;
  deltaY: number;
  /** WheelEvent.deltaMode: 0 pixels, 1 lines, 2 pages. */
  deltaMode: number;
  timeStamp: number;
  ctrlKey: boolean;
  shiftKey: boolean;
}

export interface Swipe {
  dx: number;
  dy: number;
  lastAt: number;
  /**
   * `idle` until DECIDE_PX of travel, then `swiping` or `ignored` (a scroll, or a swipe the
   * target scrolls itself) for the rest of the gesture. `fired` swallows the momentum tail.
   */
  phase: 'idle' | 'swiping' | 'ignored' | 'fired';
}

export const IDLE_SWIPE: Swipe = { dx: 0, dy: 0, lastAt: -Infinity, phase: 'idle' };

export interface SwipeStep {
  swipe: Swipe;
  /** Set on the event that crosses the threshold. */
  fire: SwipeDirection | null;
  /** While swiping: which way and how far toward the threshold (0–1), for feedback. */
  progress: { direction: SwipeDirection; amount: number } | null;
  /** The event is part of a swipe; the caller should preventDefault it. */
  consume: boolean;
}

export interface SwipeContext {
  /** Whether `direction` has anywhere to go. */
  canGo: (direction: SwipeDirection) => boolean;
  /** Whether the element under the pointer can scroll horizontally that way (-1 left, 1 right). */
  canScrollX: (sign: -1 | 1) => boolean;
}

const scale = (delta: number, mode: number) =>
  mode === 1 ? delta * LINE_PX : mode === 2 ? delta * PAGE_PX : delta;

/** Scrolling toward the left edge goes back, as in browsers: fingers move right. */
const directionOf = (dx: number): SwipeDirection => (dx < 0 ? 'back' : 'forward');

/** Feeds one wheel event into the gesture `s`. */
export function trackSwipe(s: Swipe, e: WheelSample, ctx: SwipeContext): SwipeStep {
  const dx = scale(e.deltaX, e.deltaMode);
  const dy = scale(e.deltaY, e.deltaMode);
  const start = e.timeStamp - s.lastAt > GESTURE_GAP_MS ? IDLE_SWIPE : s;
  const swipe: Swipe = { ...start, dx: start.dx + dx, dy: start.dy + dy, lastAt: e.timeStamp };
  const none = { fire: null, progress: null };

  // Ctrl is pinch-zoom; Shift turns a mouse wheel horizontal. Neither is a swipe.
  if (e.ctrlKey || e.shiftKey)
    return { swipe: { ...swipe, phase: 'ignored' }, ...none, consume: false };

  switch (swipe.phase) {
    case 'ignored':
      return { swipe, ...none, consume: false };
    case 'fired':
      return { swipe, ...none, consume: Math.abs(dx) > Math.abs(dy) };
    case 'idle': {
      if (Math.abs(swipe.dx) + Math.abs(swipe.dy) < DECIDE_PX) {
        return { swipe, ...none, consume: Math.abs(dx) > Math.abs(dy) };
      }
      const horizontal = Math.abs(swipe.dx) > 2 * Math.abs(swipe.dy);
      const direction = directionOf(swipe.dx);
      const swiping = horizontal && !ctx.canScrollX(swipe.dx < 0 ? -1 : 1) && ctx.canGo(direction);
      if (!swiping) return { swipe: { ...swipe, phase: 'ignored' }, ...none, consume: false };
      return advance({ ...swipe, phase: 'swiping' }, ctx);
    }
    case 'swiping':
      return advance(swipe, ctx);
    default: {
      const unreachable: never = swipe.phase;
      return unreachable;
    }
  }
}

function advance(swipe: Swipe, ctx: SwipeContext): SwipeStep {
  const direction = directionOf(swipe.dx);
  // Swiping back past the start toward a direction with no history shows and fires nothing.
  if (!ctx.canGo(direction)) return { swipe, fire: null, progress: null, consume: true };
  const amount = Math.min(Math.abs(swipe.dx) / SWIPE_THRESHOLD, 1);
  if (amount < 1) return { swipe, fire: null, progress: { direction, amount }, consume: true };
  return {
    swipe: { ...swipe, phase: 'fired' },
    fire: direction,
    progress: { direction, amount },
    consume: true,
  };
}
