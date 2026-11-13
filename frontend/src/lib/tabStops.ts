const FOCUSABLE = 'button, input, select, textarea, a[href], [tabindex]';

/**
 * Elements Tab visits inside `root`, in document order. A radio group is one stop: its checked
 * radio, else its first. Needed because WKWebView skips buttons unless macOS keyboard
 * navigation is on, which would strand focus in button-only dialogs.
 */
export function tabStops(root: HTMLElement): HTMLElement[] {
  const all = [...root.querySelectorAll<HTMLElement>(FOCUSABLE)];
  const radios = all.filter(
    (el): el is HTMLInputElement => el instanceof HTMLInputElement && el.type === 'radio'
  );

  return all.filter((el) => {
    if (el.tabIndex < 0 || el.matches(':disabled') || el.closest('[hidden], [inert]')) return false;
    if (!(el instanceof HTMLInputElement)) return true;
    if (el.type === 'hidden') return false;
    if (el.type !== 'radio' || el.name === '') return true;

    const group = radios.filter((r) => r.name === el.name);
    return el === (group.find((r) => r.checked) ?? group[0]);
  });
}

function sameRadioGroup(stop: HTMLElement, other: Element): boolean {
  return (
    stop instanceof HTMLInputElement &&
    other instanceof HTMLInputElement &&
    stop.type === 'radio' &&
    other.type === 'radio' &&
    stop.name !== '' &&
    stop.name === other.name &&
    stop.form === other.form
  );
}

/** The stop after (or before) `current`, wrapping; the first (or last) when focus is elsewhere. */
export function nextTabStop(
  root: HTMLElement,
  current: Element | null,
  backward: boolean
): HTMLElement | undefined {
  const stops = tabStops(root);
  if (stops.length === 0) return undefined;

  const at = current ? stops.findIndex((el) => el === current || sameRadioGroup(el, current)) : -1;
  if (at === -1) return backward ? stops.at(-1) : stops[0];
  return stops[(at + (backward ? -1 : 1) + stops.length) % stops.length];
}
