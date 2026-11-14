import { create } from 'zustand';
import * as api from '../api';
import { toAppError } from '../api/errors';
import type { Settings } from '../api/types';
import { useUi } from './ui';

interface SettingsState {
  settings: Settings | null;
  load: () => Promise<void>;
  /** Applies the patch optimistically, then reconciles with what the backend stored. */
  save: (patch: Partial<Settings>) => Promise<void>;
}

export const useSettings = create<SettingsState>()((set, get) => ({
  settings: null,

  async load() {
    try {
      set({ settings: await api.settings.get() });
    } catch (e) {
      useUi.getState().showError('Couldn’t load settings', toAppError(e));
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
/** On until settings load: it's the default. */
export const selectSwipeNavigation = (s: SettingsState): boolean =>
  s.settings?.swipeNavigation ?? true;
