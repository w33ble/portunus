/** Mirrors internal/services/errors.go ErrorKind; Go does not export it to the bindings. */
export const ERROR_KINDS = [
  'not_exist',
  'exist',
  'permission',
  'unsupported',
  'not_connected',
  'hostkey_mismatch',
  'canceled',
  'other',
] as const;

export type ErrorKind = (typeof ERROR_KINDS)[number];

export class AppError extends Error {
  readonly kind: ErrorKind;

  constructor(kind: ErrorKind, message: string) {
    super(message);
    this.name = 'AppError';
    this.kind = kind;
  }
}

function isErrorKind(value: unknown): value is ErrorKind {
  return ERROR_KINDS.some((k) => k === value);
}

/** Wails rejects cancelled calls with CancelError (name "CancelError"). Matching by name keeps the runtime out of unit tests. */
function isCancelError(error: unknown): boolean {
  return error instanceof Error && error.name === 'CancelError';
}

function causeOf(error: unknown): { kind: ErrorKind; message: string } | null {
  if (typeof error !== 'object' || error === null || !('cause' in error)) return null;
  const cause = error.cause;
  if (typeof cause !== 'object' || cause === null) return null;
  const kind: unknown = 'kind' in cause ? cause.kind : undefined;
  const message: unknown = 'message' in cause ? cause.message : undefined;
  if (typeof message !== 'string') return null;
  return { kind: isErrorKind(kind) ? kind : 'other', message };
}

/** Parses whatever a rejected binding call threw into a typed error. */
export function toAppError(error: unknown): AppError {
  if (error instanceof AppError) return error;
  if (isCancelError(error)) return new AppError('canceled', 'Canceled');
  const cause = causeOf(error);
  if (cause) return new AppError(cause.kind, cause.message);
  if (error instanceof Error) return new AppError('other', error.message);
  return new AppError('other', typeof error === 'string' ? error : 'Unknown error');
}

export function isCanceled(error: unknown): boolean {
  return toAppError(error).kind === 'canceled';
}

/** Short, user-facing text for a failed operation. */
export function describeError(error: AppError): string {
  switch (error.kind) {
    case 'permission':
      return `Permission denied. ${error.message}`;
    case 'not_exist':
      return `Not found. ${error.message}`;
    case 'exist':
      return `Already exists. ${error.message}`;
    case 'not_connected':
      return `Not connected. ${error.message}`;
    case 'unsupported':
      return `Not supported here. ${error.message}`;
    case 'hostkey_mismatch':
    case 'canceled':
    case 'other':
      return error.message;
    default: {
      const unreachable: never = error.kind;
      return unreachable;
    }
  }
}
