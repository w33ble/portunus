import { useEffect, useState } from 'react';
import * as api from '../../api';
import { type AppError, toAppError } from '../../api/errors';
import type { DirSummary, Entry } from '../../api/types';

/** Arrowing through a list only loads the entry the cursor settles on. */
export const PREVIEW_DELAY_MS = 120;

export type Loaded = api.PreviewContent | { kind: 'dir'; summary: DirSummary };

export type PreviewState =
  | { status: 'empty' }
  | { status: 'loading' }
  | { status: 'ready'; content: Loaded }
  | { status: 'error'; error: AppError };

const EMPTY: PreviewState = { status: 'empty' };
const LOADING: PreviewState = { status: 'loading' };

function load(connId: string, path: string, isDir: boolean, signal: AbortSignal): Promise<Loaded> {
  if (isDir) {
    return api.files.summarize(connId, path, signal).then((summary) => ({ kind: 'dir', summary }));
  }
  return api.preview.load(connId, path, { signal, pdfViewer: navigator.pdfViewerEnabled });
}

/**
 * Loads the preview of `entry`. Each change aborts the previous load, which closes the
 * backend read, so moving the cursor never queues downloads.
 */
export function usePreview(connId: string | null, entry: Entry | null): PreviewState {
  const path = entry?.path ?? null;
  const isDir = entry?.isDir ?? false;
  // A changed size or mtime (a refresh) reloads the same path.
  const key =
    connId !== null && entry !== null
      ? [connId, entry.path, entry.isDir, entry.size, entry.modTime].join('\n')
      : null;
  const [result, setResult] = useState<{ key: string; state: PreviewState } | null>(null);

  useEffect(() => {
    if (key === null || connId === null || path === null) return;
    const controller = new AbortController();
    const settle = (state: PreviewState) => {
      if (!controller.signal.aborted) setResult({ key, state });
    };
    const timer = setTimeout(() => {
      load(connId, path, isDir, controller.signal).then(
        (content) => settle({ status: 'ready', content }),
        (e: unknown) => settle({ status: 'error', error: toAppError(e) })
      );
    }, PREVIEW_DELAY_MS);
    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  }, [key, connId, path, isDir]);

  if (key === null) return EMPTY;
  return result?.key === key ? result.state : LOADING;
}
