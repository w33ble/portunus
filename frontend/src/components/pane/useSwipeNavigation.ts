import { type RefObject, useEffect } from 'react';
import {
  GESTURE_GAP_MS,
  IDLE_SWIPE,
  type SwipeDirection,
  type SwipeStep,
  trackSwipe,
} from '../../lib/swipe';
import type { PaneId } from '../../stores/paneTypes';
import { usePanes } from '../../stores/panes';
import { selectSwipeNavigation, useSettings } from '../../stores/settings';

/** Whether an element between `target` and `root` can still scroll horizontally toward `sign`. */
function canScrollX(target: EventTarget | null, root: HTMLElement, sign: -1 | 1): boolean {
  for (
    let el = target instanceof Element ? target : null;
    el && root.contains(el);
    el = el.parentElement
  ) {
    if (el.scrollWidth <= el.clientWidth) continue;
    const { overflowX } = getComputedStyle(el);
    if (overflowX !== 'auto' && overflowX !== 'scroll') continue;
    if (sign < 0 ? el.scrollLeft > 0 : el.scrollLeft + el.clientWidth < el.scrollWidth - 1) {
      return true;
    }
  }
  return false;
}

function hasHistory(paneId: PaneId, direction: SwipeDirection): boolean {
  const { history } = usePanes.getState().panes[paneId];
  return (direction === 'back' ? history.back : history.forward).length > 0;
}

/** Drawn straight on the DOM: a re-render per wheel event would re-render the file list. */
function draw(indicator: HTMLElement | null, progress: SwipeStep['progress']): void {
  if (indicator === null) return;
  if (progress === null) {
    delete indicator.dataset['direction'];
    delete indicator.dataset['armed'];
    return;
  }
  indicator.dataset['direction'] = progress.direction;
  indicator.style.setProperty('--swipe', String(progress.amount));
  if (progress.amount >= 1) indicator.dataset['armed'] = '';
  else delete indicator.dataset['armed'];
}

/**
 * Two-finger horizontal swipes over `root` go back and forward in the pane's history, with an
 * edge arrow in `indicator` that follows the swipe. Off when the setting is.
 */
export function useSwipeNavigation(
  paneId: PaneId,
  root: RefObject<HTMLElement | null>,
  indicator: RefObject<HTMLElement | null>
): void {
  const enabled = useSettings(selectSwipeNavigation);

  useEffect(() => {
    const el = root.current;
    const arrow = indicator.current;
    if (el === null || !enabled) return;
    let swipe = IDLE_SWIPE;
    let hideTimer: ReturnType<typeof setTimeout> | undefined;

    const onWheel = (event: WheelEvent) => {
      const step = trackSwipe(swipe, event, {
        canGo: (direction) => hasHistory(paneId, direction),
        canScrollX: (sign) => canScrollX(event.target, el, sign),
      });
      swipe = step.swipe;
      if (step.consume) event.preventDefault();

      // A fired swipe keeps its armed arrow until the gesture's momentum dies down.
      if (step.progress !== null || step.swipe.phase !== 'fired') draw(arrow, step.progress);
      clearTimeout(hideTimer);
      hideTimer = setTimeout(() => draw(arrow, null), GESTURE_GAP_MS);

      if (step.fire === 'back') usePanes.getState().goBack(paneId);
      else if (step.fire === 'forward') usePanes.getState().goForward(paneId);
    };

    el.addEventListener('wheel', onWheel, { passive: false });
    return () => {
      el.removeEventListener('wheel', onWheel);
      clearTimeout(hideTimer);
      draw(arrow, null);
    };
  }, [enabled, paneId, root, indicator]);
}
