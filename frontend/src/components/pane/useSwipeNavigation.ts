import { type RefObject, useEffect } from 'react';
import {
  GESTURE_GAP_MS,
  IDLE_SWIPE,
  type SwipeDirection,
  type SwipeStep,
  trackSwipe,
} from '../../lib/swipe';
import { type PaneId, parsePaneId } from '../../stores/paneTypes';
import { usePanes } from '../../stores/panes';
import { selectScrollSwipes, selectSwipeNavigation, useSettings } from '../../stores/settings';
import { useUi } from '../../stores/ui';

/** How long the arrow stays up after an OS swipe, which has no progress to follow. */
const FLASH_MS = 300;

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

function go(paneId: PaneId, direction: SwipeDirection): void {
  if (direction === 'back') usePanes.getState().goBack(paneId);
  else usePanes.getState().goForward(paneId);
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
 * The OS's own swipe gesture (macOS three-finger swipe, sent as one event): steps the pane
 * under the pointer, or the active pane when the pointer is elsewhere, and flashes its arrow.
 */
export function swipeFromOs(direction: SwipeDirection): void {
  if (!selectSwipeNavigation(useSettings.getState())) return;
  const hovered = document.querySelector<HTMLElement>('section[data-pane]:hover');
  const paneId = parsePaneId(hovered?.dataset['pane'] ?? '') ?? useUi.getState().activePane;
  if (!hasHistory(paneId, direction)) return;

  const arrow = document.querySelector<HTMLElement>(
    `section[data-pane="${paneId}"] [data-swipe-arrow]`
  );
  draw(arrow, { direction, amount: 1 });
  setTimeout(() => draw(arrow, null), FLASH_MS);
  go(paneId, direction);
}

/**
 * Two-finger horizontal swipes over `root` go back and forward in the pane's history, with an
 * edge arrow in `indicator` that follows the swipe. Off when the setting is, or when the OS
 * keeps two-finger scrolls for scrolling (macOS with "Swipe between pages" on three fingers).
 */
export function useSwipeNavigation(
  paneId: PaneId,
  root: RefObject<HTMLElement | null>,
  indicator: RefObject<HTMLElement | null>
): void {
  const enabled = useSettings(selectScrollSwipes);

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

      if (step.fire !== null) go(paneId, step.fire);
    };

    el.addEventListener('wheel', onWheel, { passive: false });
    return () => {
      el.removeEventListener('wheel', onWheel);
      clearTimeout(hideTimer);
      draw(arrow, null);
    };
  }, [enabled, paneId, root, indicator]);
}
