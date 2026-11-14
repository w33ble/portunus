import { create } from 'zustand';
import * as api from '../api';
import { toAppError } from '../api/errors';
import type { Settings } from '../api/types';
import { useUi } from './ui';

interface SettingsState {
  settings: Settings | null;
  /** The OS treats two-finger sideways scrolls as swipes (macOS "Swipe between pages"). */
  swipeFromScroll: boolean;
  load: () => Promise<void>;
  /** Rereads OS preferences that can change while the app runs. */
  loadSystem: () => Promise<void>;
  /** Applies the patch optimistically, then reconciles with what the backend stored. */
  save: (patch: Partial<Settings>) => Promise<void>;
}

export const useSettings = create<SettingsState>()((set, get) => ({
  settings: null,
  swipeFromScroll: true,

  async load() {
    try {
      set({ settings: await api.settings.get() });
    } catch (e) {
      useUi.getState().showError('Couldn’t load settings', toAppError(e));
    }
  },

  async loadSystem() {
    try {
      set({ swipeFromScroll: await api.settings.swipeFromScroll() });
    } catch {
      // Keep the last known value; this only tunes gesture handling.
    }
  },

  async save(patch) {
    const previous = get().settings;
    if (previous === null) return;
    set({ settings: { ...previous, ...patch } });
    try {
      set({ settings: await api.settings.save({ ...previous, ...patch }) });
    } catch (e) {
      set({ settings: previous });
      useUi.getState().showError('Couldn’t save settings', toAppError(e));
    }
  },
}));

export const selectShowHidden = (s: SettingsState): boolean => s.settings?.showHidden ?? false;
/** On until settings load: it's the default. Covers the OS's own swipe gesture. */
export const selectSwipeNavigation = (s: SettingsState): boolean =>
  s.settings?.swipeNavigation ?? true;
/** Two-finger scrolls only swipe when the OS says they should, as in Safari and Finder. */
export const selectScrollSwipes = (s: SettingsState): boolean =>
  selectSwipeNavigation(s) && s.swipeFromScroll;
