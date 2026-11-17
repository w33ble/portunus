import { describe, expect, it } from 'vitest';
import { SYSTEM_THEME, resolveTheme } from './theme';

describe('resolveTheme', () => {
  it.each([
    [SYSTEM_THEME, false, 'light'],
    [SYSTEM_THEME, true, 'dark'],
    ['nord-dark', false, 'nord-dark'],
    ['solarized-light', true, 'solarized-light'],
    ['a-theme-that-was-removed', true, 'dark'],
  ])('%s with the OS dark=%s applies %s', (setting, systemDark, want) => {
    expect(resolveTheme(setting, systemDark)).toBe(want);
  });
});
