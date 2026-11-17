import { resolveTheme } from '../lib/theme';
import { useSettings } from './settings';

const DARK_QUERY = '(prefers-color-scheme: dark)';

/**
 * Keeps the root's data-theme on the saved theme, and on the OS appearance while that's System.
 * Until settings load it leaves the attribute alone: /theme.js already set it before first paint.
 */
export function followTheme(): () => void {
  const media = window.matchMedia(DARK_QUERY);
  const apply = () => {
    const theme = useSettings.getState().settings?.theme;
    if (theme === undefined) return;
    document.documentElement.dataset['theme'] = resolveTheme(theme, media.matches);
  };

  const unsubscribe = useSettings.subscribe(apply);
  media.addEventListener('change', apply);
  apply();

  return () => {
    unsubscribe();
    media.removeEventListener('change', apply);
  };
}
