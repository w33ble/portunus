import { describe, expect, it } from 'vitest';
import { AppError, isCanceled, toAppError } from './errors';

function bindingError(cause: unknown, message = 'rpc failed'): Error {
  const err = new Error(message);
  err.cause = cause;
  return err;
}

describe('toAppError', () => {
  it('parses the marshaled cause', () => {
    const err = toAppError(bindingError({ kind: 'permission', message: 'open /root: denied' }));
    expect(err.kind).toBe('permission');
    expect(err.message).toBe('open /root: denied');
  });

  it('preserves host key mismatch', () => {
    expect(toAppError(bindingError({ kind: 'hostkey_mismatch', message: 'changed' })).kind).toBe(
      'hostkey_mismatch'
    );
  });

  it('maps unknown kinds to other but keeps the message', () => {
    const err = toAppError(bindingError({ kind: 'brand_new', message: 'boom' }));
    expect(err.kind).toBe('other');
    expect(err.message).toBe('boom');
  });

  it('falls back to the error message without a cause', () => {
    expect(toAppError(new Error('network down'))).toMatchObject({
      kind: 'other',
      message: 'network down',
    });
  });

  it('ignores a malformed cause', () => {
    expect(toAppError(bindingError('just a string', 'fallback')).message).toBe('fallback');
    expect(toAppError(bindingError({ kind: 'exist' }, 'fallback')).message).toBe('fallback');
  });

  it('treats Wails cancellation as canceled', () => {
    const cancelled = new Error('Promise cancelled.');
    cancelled.name = 'CancelError';
    expect(toAppError(cancelled).kind).toBe('canceled');
    expect(isCanceled(cancelled)).toBe(true);
  });

  it('is idempotent and handles non-errors', () => {
    const e = new AppError('exist', 'x');
    expect(toAppError(e)).toBe(e);
    expect(toAppError('plain').message).toBe('plain');
    expect(toAppError(undefined).kind).toBe('other');
  });
});
