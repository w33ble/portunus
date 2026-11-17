import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Settings } from '../api/types';
import { useSettings } from './settings';
import { followTheme } from './theme';

vi.mock('../api', async () => (await import('./testApi')).createApiMock());

const base: Settings = {
  globalTransfers: 1,
  perConnectionTransfers: 1,
  loadAppleKeychain: true,
  showHidden: false,
  swipeNavigation: true,
  theme: 'system',
};

/** A prefers-color-scheme query whose answer the test flips, like the OS appearance changing. */
function fakeDarkQuery(initial: boolean) {
  const listeners = new Set<() => void>();
  const query = {
    matches: initial,
    addEventListener: (_: string, fn: () => void) => listeners.add(fn),
    removeEventListener: (_: string, fn: () => void) => listeners.delete(fn),
  };
  vi.spyOn(window, 'matchMedia').mockReturnValue(query as unknown as MediaQueryList);
  return {
    set(dark: boolean) {
      query.matches = dark;
      for (const fn of listeners) fn();
    },
    listening: () => listeners.size,
  };
}

const applied = () => document.documentElement.dataset['theme'];
const setTheme = (theme: string) => useSettings.setState({ settings: { ...base, theme } });

let stop: () => void = () => {};

beforeEach(() => {
  useSettings.setState(useSettings.getInitialState(), true);
  document.documentElement.dataset['theme'] = 'nord-dark';
});

afterEach(() => {
  stop();
  vi.restoreAllMocks();
});

describe('followTheme', () => {
  it('keeps the theme the startup script applied until settings load', () => {
    fakeDarkQuery(false);
    stop = followTheme();
    expect(applied()).toBe('nord-dark');

    setTheme('catppuccin-latte');
    expect(applied()).toBe('catppuccin-latte');
  });

  it('follows the OS appearance live while the setting is System', () => {
    const os = fakeDarkQuery(true);
    stop = followTheme();
    setTheme('system');
    expect(applied()).toBe('dark');

    os.set(false);
    expect(applied()).toBe('light');
  });

  it('ignores the OS appearance once a theme is picked', () => {
    const os = fakeDarkQuery(false);
    stop = followTheme();
    setTheme('tokyo-night');
    os.set(true);
    expect(applied()).toBe('tokyo-night');
  });

  it('stops listening when cleaned up', () => {
    const os = fakeDarkQuery(false);
    stop = followTheme();
    setTheme('system');
    stop();
    stop = () => {};

    os.set(true);
    setTheme('nord-light');
    expect(applied()).toBe('light');
    expect(os.listening()).toBe(0);
  });
});
