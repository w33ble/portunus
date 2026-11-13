export const IS_MAC = typeof navigator !== 'undefined' && /Mac/i.test(navigator.platform);

/** True for targets where arrow keys and modifiers edit text. */
export function isEditable(target: EventTarget | null): boolean {
  return (
    target instanceof HTMLElement &&
    (target.isContentEditable ||
      target instanceof HTMLInputElement ||
      target instanceof HTMLTextAreaElement)
  );
}
