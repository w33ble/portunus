import { describe, expect, it, vi } from 'vitest';
import { deferred } from '../stores/testApi';
import { loadPreview, previewUrl } from './preview';

function fetchReturning(body: BodyInit | null, init: ResponseInit) {
  return vi.fn<typeof fetch>(async () => new Response(body, init));
}

function load(path: string, fetchFn: typeof fetch, opts: { pdfViewer?: boolean } = {}) {
  return loadPreview('ssh:box', path, {
    signal: new AbortController().signal,
    pdfViewer: opts.pdfViewer ?? true,
    fetch: fetchFn,
  });
}

describe('previewUrl', () => {
  it('encodes the connection and path as query parameters', () => {
    expect(previewUrl('ssh:me@host:22', '/a b/c&d.txt')).toBe(
      '/preview?conn=ssh%3Ame%40host%3A22&path=%2Fa+b%2Fc%26d.txt'
    );
  });
});

describe('loadPreview', () => {
  it('returns text and whether it was truncated', async () => {
    const fetchFn = fetchReturning('héllo', {
      headers: { 'X-Preview-Kind': 'text', 'X-Preview-Truncated': 'true' },
    });
    await expect(load('/logs/app.log', fetchFn)).resolves.toEqual({
      kind: 'text',
      text: 'héllo',
      bytes: 6,
      truncated: true,
    });
    expect(fetchFn.mock.calls[0]?.[1]?.method).toBe('GET');
  });

  it('returns image bytes as a blob', async () => {
    const fetchFn = fetchReturning(new Uint8Array([137, 80, 78, 71]), {
      headers: { 'X-Preview-Kind': 'image', 'Content-Type': 'image/png' },
    });
    const result = await load('/p/pixel.png', fetchFn);
    expect(result.kind).toBe('image');
    if (result.kind === 'image') expect(result.blob.size).toBe(4);
  });

  it.each([
    ['binary', 'binary'],
    ['too_large', 'too_large'],
  ])('maps a 204 with reason %s to metadata only', async (header, reason) => {
    const fetchFn = fetchReturning(null, { status: 204, headers: { 'X-Preview-Reason': header } });
    await expect(load('/x/blob', fetchFn)).resolves.toEqual({ kind: 'none', reason });
  });

  it('only checks a PDF with HEAD and hands its URL to the viewer', async () => {
    const fetchFn = fetchReturning(null, { headers: { 'X-Preview-Kind': 'pdf' } });
    await expect(load('/docs/Report.PDF', fetchFn)).resolves.toEqual({
      kind: 'pdf',
      url: previewUrl('ssh:box', '/docs/Report.PDF'),
    });
    expect(fetchFn.mock.calls[0]?.[1]?.method).toBe('HEAD');
  });

  it('skips the request for a PDF when the webview has no PDF viewer', async () => {
    const fetchFn = fetchReturning(null, {});
    await expect(load('/docs/r.pdf', fetchFn, { pdfViewer: false })).resolves.toEqual({
      kind: 'none',
      reason: 'no_viewer',
    });
    expect(fetchFn).not.toHaveBeenCalled();
  });

  it('rejects with the AppError the route sent', async () => {
    const fetchFn = fetchReturning(JSON.stringify({ kind: 'not_connected', message: 'gone' }), {
      status: 404,
      headers: { 'Content-Type': 'application/json' },
    });
    await expect(load('/a.txt', fetchFn)).rejects.toMatchObject({
      kind: 'not_connected',
      message: 'gone',
    });
  });

  it('rejects as canceled once the signal aborts', async () => {
    const controller = new AbortController();
    const fetchFn = vi.fn<typeof fetch>((_url, init) => {
      const response = deferred<Response>();
      init?.signal?.addEventListener('abort', () =>
        response.reject(new DOMException('Aborted', 'AbortError'))
      );
      return response.promise;
    });
    const pending = loadPreview('local', '/a.txt', {
      signal: controller.signal,
      pdfViewer: true,
      fetch: fetchFn,
    });
    controller.abort();
    await expect(pending).rejects.toMatchObject({ kind: 'canceled' });
  });
});
