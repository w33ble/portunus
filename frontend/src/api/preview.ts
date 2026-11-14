import { AppError, toAppError } from './errors';

/** Mirrors internal/services/preview.go: the route and its response headers. */
const ROUTE = '/preview';
const KIND_HEADER = 'X-Preview-Kind';
const TRUNCATED_HEADER = 'X-Preview-Truncated';
const REASON_HEADER = 'X-Preview-Reason';

/** Why a file shows metadata only. `no_viewer` and `unreadable` are decided in the webview. */
export type NoPreviewReason = 'binary' | 'too_large' | 'no_viewer' | 'unreadable';

export type PreviewContent =
  /** `bytes` is how much of the file `text` holds: all of it unless `truncated`. */
  | { kind: 'text'; text: string; bytes: number; truncated: boolean }
  | { kind: 'image'; blob: Blob }
  /** Loaded by the webview's own PDF viewer straight from `url`. */
  | { kind: 'pdf'; url: string }
  | { kind: 'none'; reason: NoPreviewReason };

export function previewUrl(connId: string, path: string): string {
  return `${ROUTE}?${new URLSearchParams({ conn: connId, path }).toString()}`;
}

export interface PreviewOptions {
  signal: AbortSignal;
  /** Whether the webview can show PDFs (navigator.pdfViewerEnabled). */
  pdfViewer: boolean;
  fetch?: typeof fetch;
}

/**
 * Fetches what the preview of `path` shows. PDFs only get a HEAD here: the viewer loads them
 * itself. Aborting `signal` stops the backend read and rejects with a `canceled` AppError.
 */
export async function loadPreview(
  connId: string,
  path: string,
  { signal, pdfViewer, fetch: fetchFn = fetch }: PreviewOptions
): Promise<PreviewContent> {
  const pdf = path.toLowerCase().endsWith('.pdf');
  if (pdf && !pdfViewer) return { kind: 'none', reason: 'no_viewer' };

  const url = previewUrl(connId, path);
  try {
    const res = await fetchFn(url, { method: pdf ? 'HEAD' : 'GET', signal });
    if (!res.ok) throw await responseError(res);
    if (res.status === 204) {
      const reason = res.headers.get(REASON_HEADER);
      return { kind: 'none', reason: reason === 'too_large' ? 'too_large' : 'binary' };
    }

    const kind = res.headers.get(KIND_HEADER);
    switch (kind) {
      case 'text': {
        const bytes = await res.arrayBuffer();
        return {
          kind: 'text',
          text: new TextDecoder().decode(bytes),
          bytes: bytes.byteLength,
          truncated: res.headers.get(TRUNCATED_HEADER) === 'true',
        };
      }
      case 'image':
        return { kind: 'image', blob: await res.blob() };
      case 'pdf':
        return { kind: 'pdf', url };
      default:
        throw new AppError('other', `Unexpected preview kind “${kind ?? ''}”`);
    }
  } catch (e) {
    if (signal.aborted) throw new AppError('canceled', 'Canceled');
    throw toAppError(e);
  }
}

/** Error responses carry the same {kind, message} body as rejected binding calls. */
async function responseError(res: Response): Promise<AppError> {
  try {
    return toAppError({ cause: await res.json() });
  } catch {
    return new AppError('other', `Preview failed (HTTP ${res.status})`);
  }
}
