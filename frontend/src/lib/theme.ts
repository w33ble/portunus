/** The setting value that follows the OS appearance. */
export const SYSTEM_THEME = 'system';

export interface ThemeInfo {
  /** Matches a `[data-theme]` block in src/themes and the catalog in internal/theme. */
  id: string;
  name: string;
  dark: boolean;
}

/** Built-in themes, light then dark for each family. */
export const THEMES: readonly ThemeInfo[] = [
  { id: 'light', name: 'Portunus Light', dark: false },
  { id: 'dark', name: 'Portunus Dark', dark: true },
  { id: 'solarized-light', name: 'Solarized Light', dark: false },
  { id: 'solarized-dark', name: 'Solarized Dark', dark: true },
  { id: 'nord-light', name: 'Nord Light', dark: false },
  { id: 'nord-dark', name: 'Nord Dark', dark: true },
  { id: 'catppuccin-latte', name: 'Catppuccin Latte', dark: false },
  { id: 'catppuccin-mocha', name: 'Catppuccin Mocha', dark: true },
  { id: 'everforest-light', name: 'Everforest Light', dark: false },
  { id: 'everforest-dark', name: 'Everforest Dark', dark: true },
  { id: 'tokyo-night-day', name: 'Tokyo Night Day', dark: false },
  { id: 'tokyo-night', name: 'Tokyo Night', dark: true },
];

/** The theme a setting applies: a built-in ID as is, anything else follows the OS. */
export function resolveTheme(setting: string, systemDark: boolean): string {
  if (THEMES.some((t) => t.id === setting)) return setting;
  return systemDark ? 'dark' : 'light';
}
