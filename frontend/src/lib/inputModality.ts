/** Keys that mean the user is moving focus or selection with the keyboard. */
const NAVIGATION_KEYS: Record<string, true> = {
  Tab: true,
  ArrowUp: true,
  ArrowDown: true,
  ArrowLeft: true,
  ArrowRight: true,
  Home: true,
  End: true,
  PageUp: true,
  PageDown: true,
};

/**
 * Mirrors the last input modality onto `root.dataset.modality` ("pointer" until a navigation
 * key is pressed). Focus rings on surfaces focused programmatically, such as the active pane on
 * launch, key off this, since browsers treat script focus as :focus-visible. Returns the cleanup.
 */
export function trackInputModality(root: HTMLElement = document.documentElement): () => void {
  root.dataset['modality'] = 'pointer';

  const onKeyDown = (event: KeyboardEvent) => {
    if (NAVIGATION_KEYS[event.key]) root.dataset['modality'] = 'keyboard';
  };
  const onPointerDown = () => {
    root.dataset['modality'] = 'pointer';
  };

  window.addEventListener('keydown', onKeyDown, true);
  window.addEventListener('pointerdown', onPointerDown, true);
  return () => {
    window.removeEventListener('keydown', onKeyDown, true);
    window.removeEventListener('pointerdown', onPointerDown, true);
  };
}
