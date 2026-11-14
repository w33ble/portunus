import { beforeEach, describe, expect, it, vi } from 'vitest';
import * as api from '../api';
import type { Settings } from '../api/types';
import { useSettings } from './settings';
import { useUi } from './ui';

vi.mock('../api', async () => (await import('./testApi')).createApiMock());

const base: Settings = {
  globalTransfers: 1,
  perConnectionTransfers: 1,
  loadAppleKeychain: true,
  showHidden: false,
  swipeNavigation: true,
};

beforeEach(() => {
  vi.clearAllMocks();
  useSettings.setState(useSettings.getInitialState(), true);
  useUi.setState(useUi.getInitialState(), true);
});

describe('useSettings', () => {
  it('applies a patch immediately and keeps what the backend stored', async () => {
    vi.mocked(api.settings.get).mockResolvedValue(base);
    await useSettings.getState().load();

    // Backend normalizes: per-connection can't exceed global.
    vi.mocked(api.settings.save).mockResolvedValue({
      ...base,
      globalTransfers: 2,
      perConnectionTransfers: 2,
    });
    const pending = useSettings.getState().save({ globalTransfers: 2, perConnectionTransfers: 5 });
    expect(useSettings.getState().settings?.perConnectionTransfers).toBe(5);
    await pending;

    expect(api.settings.save).toHaveBeenCalledWith({
      ...base,
      globalTransfers: 2,
      perConnectionTransfers: 5,
    });
    expect(useSettings.getState().settings?.perConnectionTransfers).toBe(2);
  });

  it('rolls back and reports when saving fails', async () => {
    vi.mocked(api.settings.get).mockResolvedValue(base);
    await useSettings.getState().load();
    vi.mocked(api.settings.save).mockRejectedValue(new Error('disk full'));
    await useSettings.getState().save({ showHidden: true });
    expect(useSettings.getState().settings?.showHidden).toBe(false);
    expect(useUi.getState().dialog).toMatchObject({ kind: 'error' });
  });
});
