import { describe, expect, it } from 'vitest';
import {
  GESTURE_GAP_MS,
  IDLE_SWIPE,
  SWIPE_THRESHOLD,
  type SwipeContext,
  type SwipeStep,
  type WheelSample,
  trackSwipe,
} from './swipe';

const open: SwipeContext = { canGo: () => true, canScrollX: () => false };

const wheel = (deltaX: number, deltaY = 0, more: Partial<WheelSample> = {}) => ({
  deltaX,
  deltaY,
  deltaMode: 0,
  ctrlKey: false,
  shiftKey: false,
  ...more,
});

/** Feeds events 16 ms apart (one per frame), or at their own timeStamp when given. */
function feed(
  events: Array<Omit<WheelSample, 'timeStamp'> & { timeStamp?: number }>,
  ctx = open
): SwipeStep[] {
  let swipe = IDLE_SWIPE;
  let t = 1000;
  return events.map((e) => {
    t = e.timeStamp ?? t + 16;
    const step = trackSwipe(swipe, { ...e, timeStamp: t }, ctx);
    swipe = step.swipe;
    return step;
  });
}

const fired = (steps: SwipeStep[]) => steps.map((s) => s.fire).filter((f) => f !== null);
const repeat = <T>(n: number, x: T): T[] => Array.from({ length: n }, () => x);

describe('trackSwipe', () => {
  it('goes back once horizontal travel toward the left edge reaches the threshold', () => {
    const steps = feed(repeat(SWIPE_THRESHOLD / 10, wheel(-10)));
    expect(fired(steps)).toEqual(['back']);
    expect(steps.at(-1)?.fire).toBe('back');
    expect(steps.at(-2)?.progress).toEqual({ direction: 'back', amount: 1 - 10 / SWIPE_THRESHOLD });
  });

  it('goes forward the other way', () => {
    expect(fired(feed(repeat(20, wheel(10))))).toEqual(['forward']);
  });

  it('does nothing short of the threshold', () => {
    const steps = feed(repeat(SWIPE_THRESHOLD / 10 - 1, wheel(-10)));
    expect(fired(steps)).toEqual([]);
    expect(steps.at(-1)?.consume).toBe(true);
  });

  it('ignores a gesture that starts mostly vertical, however far it then goes sideways', () => {
    const steps = feed([...repeat(3, wheel(-2, 10)), ...repeat(40, wheel(-20))]);
    expect(fired(steps)).toEqual([]);
    expect(steps.every((s) => !s.consume)).toBe(true);
  });

  it('swallows the momentum tail instead of firing again', () => {
    // 60 frames of decaying momentum after the threshold, never pausing long enough to end it.
    const tail = Array.from({ length: 60 }, (_, i) => wheel(-Math.max(1, 30 - i / 2)));
    const steps = feed([...repeat(20, wheel(-10)), ...tail]);
    expect(fired(steps)).toEqual(['back']);
    expect(steps.at(-1)?.consume).toBe(true);
  });

  it('starts a new gesture after a pause', () => {
    const first = repeat(20, wheel(-10));
    const second = [{ ...wheel(-10), timeStamp: 1000 + 20 * 16 + GESTURE_GAP_MS + 1 }];
    const steps = feed([...first, ...second, ...repeat(19, wheel(-10))]);
    expect(fired(steps)).toEqual(['back', 'back']);
  });

  it('leaves horizontal scrolling to an element that can scroll that way', () => {
    const scrollsLeft: SwipeContext = { canGo: () => true, canScrollX: (sign) => sign < 0 };
    expect(fired(feed(repeat(30, wheel(-10)), scrollsLeft))).toEqual([]);
    expect(fired(feed(repeat(30, wheel(10)), scrollsLeft))).toEqual(['forward']);
  });

  it('ignores a direction with no history', () => {
    const backOnly: SwipeContext = { canGo: (d) => d === 'back', canScrollX: () => false };
    const steps = feed(repeat(30, wheel(10)), backOnly);
    expect(fired(steps)).toEqual([]);
    expect(steps.every((s) => s.progress === null)).toBe(true);
  });

  it('counts line-mode deltas as 16 px lines', () => {
    expect(
      fired(feed(repeat(Math.ceil(SWIPE_THRESHOLD / 16), wheel(-1, 0, { deltaMode: 1 }))))
    ).toEqual(['back']);
  });

  it.each([['ctrlKey'], ['shiftKey']])('ignores wheel events with %s held', (mod) => {
    expect(fired(feed(repeat(30, wheel(-10, 0, { [mod]: true }))))).toEqual([]);
  });
});
