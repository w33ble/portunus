import { beforeEach, describe, expect, it } from 'vitest';
import { AppError } from '../api/errors';
import { useUi } from './ui';

beforeEach(() => useUi.setState(useUi.getInitialState(), true));

describe('showError', () => {
  it('opens a generic error dialog', () => {
    useUi.getState().showError('Couldn’t delete', new AppError('permission', 'denied'));
    expect(useUi.getState().dialog).toEqual({
      kind: 'error',
      title: 'Couldn’t delete',
      message: 'denied',
      hint: null,
    });
  });

  it('explains host key mismatches regardless of the caller’s title', () => {
    useUi
      .getState()
      .showError('Couldn’t connect', new AppError('hostkey_mismatch', 'known_hosts:7'));
    const { dialog } = useUi.getState();
    expect(dialog).toMatchObject({
      kind: 'error',
      title: 'Host key changed',
      message: 'known_hosts:7',
    });
    expect(dialog?.kind === 'error' && dialog.hint).toContain('different host key');
  });

  it('stays quiet for cancellations', () => {
    useUi.getState().showError('x', new AppError('canceled', 'Canceled'));
    expect(useUi.getState().dialog).toBeNull();
  });
});
