import { act } from 'react';
import { type Root, createRoot } from 'react-dom/client';
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import * as api from './api';
import { JobKind, State } from './api/enums';
import type { JobView, Update } from './api/types';
import App from './App';
import { ENTRIES_MIME, encodeDragPayload } from './lib/dragData';
import { usePanes } from './stores/panes';
import { usePrompts } from './stores/prompts';
import { useSettings } from './stores/settings';
import { useTransfers } from './stores/transfers';
import { PREVIEW_DELAY_MS } from './components/preview/usePreview';
import { GESTURE_GAP_MS } from './lib/swipe';
import { deferred, entry, localConnection } from './stores/testApi';
import { useUi } from './stores/ui';
import { useConnections } from './stores/connections';

vi.mock('./api', async () => (await import('./stores/testApi')).createApiMock());

// The real events module needs the Wails runtime; capture its subscriptions instead.
const { handlers, on } = vi.hoisted(() => {
  const registry = new Map<string, (data: never) => void>();
  const subscribe = (name: string) => (cb: (data: never) => void) => {
    registry.set(name, cb);
    return () => registry.delete(name);
  };
  return { handlers: registry, on: subscribe };
});

vi.mock('./api/events', () => ({
  events: {
    transfersUpdate: on('transfersUpdate'),
    connectionsChanged: on('connectionsChanged'),
    dropFiles: on('dropFiles'),
    promptHostKey: on('promptHostKey'),
    promptSecret: on('promptSecret'),
    promptConflict: on('promptConflict'),
    promptResolved: on('promptResolved'),
    swipe: on('swipe'),
  },
}));

function emit(name: string, data: unknown) {
  act(() => handlers.get(name)?.(data as never));
}

const settings = {
  globalTransfers: 2,
  perConnectionTransfers: 1,
  loadAppleKeychain: true,
  showHidden: false,
  swipeNavigation: true,
};

function showModal(this: HTMLDialogElement) {
  this.setAttribute('open', '');
}

function close(this: HTMLDialogElement) {
  this.removeAttribute('open');
}

beforeAll(() => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
  // happy-dom does no layout; give the virtualizer a viewport.
  Object.defineProperty(HTMLElement.prototype, 'offsetHeight', { configurable: true, value: 600 });
  Object.defineProperty(HTMLElement.prototype, 'offsetWidth', { configurable: true, value: 600 });
  Object.defineProperty(HTMLElement.prototype, 'clientHeight', { configurable: true, value: 600 });
  Object.defineProperty(Element.prototype, 'getBoundingClientRect', {
    configurable: true,
    value: () => ({
      width: 600,
      height: 600,
      top: 0,
      left: 0,
      right: 600,
      bottom: 600,
      x: 0,
      y: 0,
    }),
  });
  vi.stubGlobal(
    'ResizeObserver',
    class {
      constructor(private cb: ResizeObserverCallback) {}
      observe(target: Element) {
        this.cb(
          [{ target, contentRect: target.getBoundingClientRect() } as ResizeObserverEntry],
          this as never
        );
      }
      unobserve() {}
      disconnect() {}
    }
  );
  HTMLDialogElement.prototype.showModal ??= showModal;
  HTMLDialogElement.prototype.close ??= close;
});

let root: Root;
let host: HTMLElement;

async function flush() {
  await act(async () => {
    await new Promise((r) => setTimeout(r, 120));
  });
}

beforeEach(async () => {
  vi.clearAllMocks();
  handlers.clear();
  usePanes.setState(usePanes.getInitialState(), true);
  useUi.setState(useUi.getInitialState(), true);
  useTransfers.setState(useTransfers.getInitialState(), true);
  usePrompts.setState(usePrompts.getInitialState(), true);
  useSettings.setState(useSettings.getInitialState(), true);
  useConnections.setState(useConnections.getInitialState(), true);
  vi.mocked(api.settings.get).mockResolvedValue(settings);
  vi.mocked(api.settings.swipeFromScroll).mockResolvedValue(true);
  vi.mocked(api.connections.list).mockResolvedValue([localConnection()]);
  vi.mocked(api.connections.sshHosts).mockResolvedValue([{ alias: 'prod', source: 'c:1' }]);
  vi.mocked(api.connections.sshAvailable).mockResolvedValue('');
  vi.mocked(api.connections.secretsSessionOnly).mockResolvedValue(false);
  vi.mocked(api.connections.s3Profiles).mockResolvedValue([]);
  vi.mocked(api.connections.connect).mockResolvedValue(localConnection());
  vi.mocked(api.transfers.jobs).mockResolvedValue([]);
  vi.mocked(api.prompts.pending).mockResolvedValue({ hostKeys: [], secrets: [], conflicts: [] });
  vi.mocked(api.files.list).mockImplementation((_conn, _dir, onPage) => {
    onPage([entry('Documents', true), entry('notes.txt'), entry('.hidden')]);
    return { done: Promise.resolve(), cancel: vi.fn() };
  });

  host = document.createElement('div');
  document.body.append(host);
  root = createRoot(host);
  await act(async () => root.render(<App />));
  await flush();
});

afterEach(() => {
  act(() => root.unmount());
  host.remove();
});

const rowNames = (pane: 'left' | 'right') =>
  [...host.querySelectorAll(`[data-pane="${pane}"] [role=option]`)].map((r) => r.textContent ?? '');

/** Dispatches an in-app drag of an S3 file onto el. happy-dom ignores DragEvent's dataTransfer init. */
async function dropOn(el: Element | null | undefined) {
  const dataTransfer = new DataTransfer();
  dataTransfer.setData(
    ENTRIES_MIME,
    encodeDragPayload({ connId: 's3:p', paths: ['/bucket/a.txt'] })
  );
  await act(async () => {
    for (const type of ['dragover', 'drop']) {
      const event = new DragEvent(type, { bubbles: true, cancelable: true });
      Object.defineProperty(event, 'dataTransfer', { value: dataTransfer });
      el?.dispatchEvent(event);
    }
  });
}

const documentsRow = () =>
  [...host.querySelectorAll<HTMLElement>('[data-pane="left"] [role=option]')].find((r) =>
    r.textContent?.includes('Documents')
  );

const leftList = () => host.querySelector<HTMLElement>('[data-pane="left"] [data-filelist]');
const key = (el: Element | null, init: KeyboardEventInit) =>
  act(async () => {
    el?.dispatchEvent(new KeyboardEvent('keydown', { bubbles: true, cancelable: true, ...init }));
  });
const filterField = () =>
  host.querySelector<HTMLInputElement>('[data-pane="left"] input[aria-label="Filter"]');

/** Sets an input's value the way typing does, so React's onChange fires. */
async function typeInto(input: HTMLInputElement | null, value: string) {
  await act(async () => {
    if (!input) return;
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set?.call(input, value);
    input.dispatchEvent(new Event('input', { bubbles: true }));
  });
}

const namesOf = (selector: string) =>
  [...host.querySelectorAll(`[data-pane="left"] ${selector} [title]`)].map((r) =>
    r.getAttribute('title')
  );
const selectedNames = () => namesOf('[role=option][aria-selected="true"]');
const shownNames = () => namesOf('[role=option]');

const preview = () => host.querySelector<HTMLElement>('section[data-preview]');
const previewBody = () => host.querySelector<HTMLElement>('[data-preview-body]');
const rightPane = () => host.querySelector<HTMLElement>('section[data-pane="right"]');
const settle = () =>
  act(async () => {
    await vi.advanceTimersByTimeAsync(PREVIEW_DELAY_MS);
  });
const clickRow = (name: string) =>
  act(async () =>
    [...host.querySelectorAll<HTMLElement>('[data-pane="left"] [role=option]')]
      .find((r) => r.textContent?.includes(name))
      ?.click()
  );

const leftPath = () => usePanes.getState().panes.left.path;
const mouse = (pane: 'left' | 'right', button: number) =>
  act(async () => {
    const list = host.querySelector(`[data-pane="${pane}"] [data-filelist]`);
    for (const type of ['mousedown', 'mouseup']) {
      list?.dispatchEvent(new MouseEvent(type, { button, bubbles: true, cancelable: true }));
    }
  });
/** A two-finger swipe: `n` wheel events of `deltaX` each, back to back. */
const swipe = (deltaX: number, n = 20) =>
  act(async () => {
    for (let i = 0; i < n; i++) {
      leftList()?.dispatchEvent(
        new WheelEvent('wheel', { deltaX, bubbles: true, cancelable: true })
      );
    }
  });
const intoDocuments = async () => {
  act(() => usePanes.getState().navigate('left', '/home/Documents'));
  await flush();
};

describe('App', () => {
  it('opens both panes on the local home and hides dotfiles', () => {
    expect(rowNames('left')[0]).toContain('Documents');
    expect(rowNames('left').some((t) => t.includes('notes.txt'))).toBe(true);
    expect(rowNames('left').some((t) => t.includes('.hidden'))).toBe(false);
    expect(rowNames('right')).toHaveLength(2);
    expect(host.querySelectorAll('section[data-file-drop-target]')).toHaveLength(2);
  });

  it('opens the panes even when other startup calls never return', async () => {
    act(() => root.unmount());
    usePanes.setState(usePanes.getInitialState(), true);
    const never = new Promise<never>(() => {});
    vi.mocked(api.settings.get).mockReturnValue(never);
    vi.mocked(api.connections.sshHosts).mockReturnValue(never);
    vi.mocked(api.transfers.jobs).mockReturnValue(never);
    vi.mocked(api.prompts.pending).mockReturnValue(never);

    root = createRoot(host);
    await act(async () => root.render(<App />));
    await flush();

    expect(rowNames('left').some((t) => t.includes('notes.txt'))).toBe(true);
    expect(rowNames('right').some((t) => t.includes('notes.txt'))).toBe(true);
  });

  it('shows a Transfers badge and the status bar for active jobs', async () => {
    const job: JobView = {
      id: 'j1',
      kind: JobKind.KindCopy,
      state: State.StateRunning,
      srcConn: 'local',
      dstConn: 'local',
      dstDir: '/dst',
      title: 'photo.jpg',
      walking: false,
      totalFiles: 1,
      doneFiles: 0,
      failedFiles: 0,
      skippedFiles: 0,
      totalBytes: 100,
      doneBytes: 50,
      bytesPerSec: 2048,
      etaSeconds: 30,
      active: null,
      failed: null,
      createdAt: '2026-01-01T00:00:00Z',
    };
    const update: Update = { jobs: [job], removed: null };
    emit('transfersUpdate', update);
    await flush();
    expect(host.querySelector('[role=tab] [aria-label="1 active"]')).not.toBeNull();
    expect(host.querySelector('footer')?.textContent).toContain('1 active transfer');
    expect(host.querySelector('footer')?.textContent).toContain('2.0 KB/s');

    await act(async () => {
      host.querySelectorAll<HTMLElement>('[role=tab]')[1]?.click();
    });
    expect(host.textContent).toContain('photo.jpg');
    expect(host.textContent).toContain('30s left');
  });

  it('queues backend prompts and shows the host key dialog first', async () => {
    emit('promptHostKey', {
      id: '1',
      request: {
        host: 'prod',
        address: 'prod:22',
        keyType: 'ssh-ed25519',
        fingerprint: 'SHA256:abc',
        saveTo: '/k',
      },
    });
    emit('promptSecret', {
      id: '2',
      request: { title: 'Passphrase', message: '', label: 'Passphrase', echo: false },
    });
    await flush();
    expect(host.querySelector('dialog')?.textContent).toContain('SHA256:abc');
    emit('promptResolved', '1');
    await flush();
    expect(host.querySelector('dialog')?.textContent).toContain('Passphrase');
  });

  it('opens the delete confirmation for the selection and starts a delete job', async () => {
    const list = host.querySelector<HTMLElement>('[data-pane="left"] [data-filelist]');
    const row = [...host.querySelectorAll<HTMLElement>('[data-pane="left"] [role=option]')].find(
      (r) => r.textContent?.includes('notes.txt')
    );
    await act(async () => row?.click());
    vi.mocked(api.transfers.remove).mockResolvedValue('job');
    await act(async () => {
      list?.dispatchEvent(new KeyboardEvent('keydown', { key: 'Delete', bubbles: true }));
    });
    expect(host.querySelector('dialog')?.textContent).toContain('Delete “notes.txt”?');
    const confirm = [...host.querySelectorAll<HTMLButtonElement>('dialog button')].find(
      (b) => b.textContent === 'Delete'
    );
    await act(async () => confirm?.click());
    expect(api.transfers.remove).toHaveBeenCalledWith('local', ['/home/notes.txt']);
  });

  it('F5 copies the selection into the other pane with the ask policy handled by the api layer', async () => {
    const list = host.querySelector<HTMLElement>('[data-pane="left"] [data-filelist]');
    const row = [...host.querySelectorAll<HTMLElement>('[data-pane="left"] [role=option]')].find(
      (r) => r.textContent?.includes('notes.txt')
    );
    await act(async () => row?.click());
    act(() => usePanes.getState().navigate('right', '/elsewhere'));
    vi.mocked(api.transfers.copy).mockResolvedValue('job');
    await act(async () => {
      list?.dispatchEvent(new KeyboardEvent('keydown', { key: 'F5', bubbles: true }));
    });
    expect(api.transfers.copy).toHaveBeenCalledWith({
      srcConn: 'local',
      sources: ['/home/notes.txt'],
      dstConn: 'local',
      dstDir: '/elsewhere',
    });
  });

  describe('type to find', () => {
    it('jumps to and selects the first entry starting with the typed letters', async () => {
      await key(leftList(), { key: 'n' });
      expect(selectedNames()).toEqual(['notes.txt']);
      await key(leftList(), { key: 'D' });
      expect(selectedNames()).toEqual(['notes.txt']);
    });

    it('narrows the list with ⌘F and restores it on Esc', async () => {
      await key(leftList(), { key: 'f', metaKey: true });
      await typeInto(filterField(), 'note');
      expect(shownNames()).toEqual(['notes.txt']);
      expect(host.querySelector('[data-pane="left"] mark')?.textContent).toBe('note');

      await typeInto(filterField(), 'zzz');
      expect(shownNames()).toEqual([]);
      expect(host.querySelector('[data-pane="left"]')?.textContent).toContain(
        'No matches for “zzz”'
      );

      await key(filterField(), { key: 'Escape' });
      expect(filterField()).toBeNull();
      expect(shownNames()).toEqual(['Documents', 'notes.txt']);
    });

    it('acts only on entries the filter leaves visible', async () => {
      vi.mocked(api.transfers.remove).mockResolvedValue('job');
      await key(leftList(), { key: 'a', ctrlKey: true });
      await key(leftList(), { key: 'f', ctrlKey: true });
      await typeInto(filterField(), 'doc');
      await key(leftList(), { key: 'Delete' });
      const confirm = [...host.querySelectorAll<HTMLButtonElement>('dialog button')].find(
        (b) => b.textContent === 'Delete'
      );
      await act(async () => confirm?.click());
      expect(api.transfers.remove).toHaveBeenCalledWith('local', ['/home/Documents']);
    });

    it('Enter in the filter moves to the list on the best match', async () => {
      await key(leftList(), { key: 'f', metaKey: true });
      await typeInto(filterField(), 'not');
      await key(filterField(), { key: 'Enter' });
      expect(selectedNames()).toEqual(['notes.txt']);
      expect(document.activeElement).toBe(leftList());
    });

    it('starts path entry when "/" is typed in the list', async () => {
      await key(leftList(), { key: '/' });
      expect(
        host.querySelector<HTMLInputElement>('[data-pane="left"] input[aria-label="Path"]')?.value
      ).toBe('/');
    });
  });

  describe('folder drop targets', () => {
    beforeEach(() => vi.mocked(api.transfers.copy).mockResolvedValue('job'));

    it('marks only folder names as drop targets', () => {
      const targets = [
        ...host.querySelectorAll('[data-pane="left"] [role=option] [data-file-drop-target]'),
      ];
      expect(targets.map((t) => t.getAttribute('data-path'))).toEqual(['/home/Documents']);
      expect(targets[0]?.getAttribute('data-pane')).toBe('left');
    });

    it('copies into the folder when dropped on its name', async () => {
      await dropOn(documentsRow()?.querySelector('[data-file-drop-target]'));
      expect(api.transfers.copy).toHaveBeenCalledWith(
        expect.objectContaining({ srcConn: 's3:p', dstDir: '/home/Documents' })
      );
    });

    it('copies into the current directory when dropped elsewhere on a folder row', async () => {
      await dropOn(documentsRow()?.lastElementChild);
      expect(api.transfers.copy).toHaveBeenCalledWith(
        expect.objectContaining({ srcConn: 's3:p', dstDir: '/home' })
      );
    });

    it('sends Finder drops on a folder name into that folder', async () => {
      emit('dropFiles', { paths: ['/Users/me/x.txt'], pane: 'right', dir: '/home/Documents' });
      await flush();
      expect(api.transfers.copy).toHaveBeenCalledWith(
        expect.objectContaining({ srcConn: 'local', dstConn: 'local', dstDir: '/home/Documents' })
      );
    });

    it('sends Finder drops without a folder into the pane directory', async () => {
      emit('dropFiles', { paths: ['/Users/me/x.txt'], pane: 'right', dir: '' });
      await flush();
      expect(api.transfers.copy).toHaveBeenCalledWith(
        expect.objectContaining({ srcConn: 'local', dstDir: '/home' })
      );
    });
  });

  describe('history gestures', () => {
    it('mouse back and forward buttons move through the history and restore the selection', async () => {
      await clickRow('notes.txt');
      await intoDocuments();
      expect(selectedNames()).toEqual([]);

      await mouse('left', 3);
      await flush();
      expect(leftPath()).toBe('/home');
      expect(selectedNames()).toEqual(['notes.txt']);

      await mouse('left', 4);
      await flush();
      expect(leftPath()).toBe('/home/Documents');
    });

    it('mouse buttons act on the pane under the pointer', async () => {
      await intoDocuments();
      await mouse('right', 3);
      await flush();
      expect(leftPath()).toBe('/home/Documents');
    });

    describe('swipes', () => {
      it('a swipe toward the left edge goes back, a new swipe the other way forward', async () => {
        await intoDocuments();
        await swipe(-20);
        expect(leftPath()).toBe('/home');

        // Lets the first gesture end; until then its momentum is swallowed.
        await act(() => new Promise((r) => setTimeout(r, GESTURE_GAP_MS + 50)));
        await swipe(20);
        expect(leftPath()).toBe('/home/Documents');
      });

      it('does nothing with the setting off', async () => {
        await intoDocuments();
        act(() => useSettings.setState({ settings: { ...settings, swipeNavigation: false } }));
        await swipe(-20);
        expect(leftPath()).toBe('/home/Documents');
        emit('swipe', 'back');
        await flush();
        expect(leftPath()).toBe('/home/Documents');
      });

      it('leaves two-finger scrolls alone when the OS keeps them for scrolling', async () => {
        await intoDocuments();
        act(() => useSettings.setState({ swipeFromScroll: false }));
        await swipe(-20);
        expect(leftPath()).toBe('/home/Documents');
      });

      it('follows the OS swipe gesture whatever the scroll setting', async () => {
        await intoDocuments();
        act(() => useSettings.setState({ swipeFromScroll: false }));
        emit('swipe', 'back');
        await flush();
        expect(leftPath()).toBe('/home');
        emit('swipe', 'forward');
        await flush();
        expect(leftPath()).toBe('/home/Documents');
      });
    });
  });

  describe('quick preview', () => {
    beforeEach(() => {
      vi.useFakeTimers();
      vi.mocked(api.preview.load).mockResolvedValue({
        kind: 'text',
        text: 'hello from portunus',
        bytes: 19,
        truncated: false,
      });
      vi.mocked(api.files.summarize).mockResolvedValue({
        dirs: 2,
        files: 3,
        size: 2048,
        partial: false,
      });
    });
    afterEach(() => vi.useRealTimers());

    it('Space shows the cursor file in place of the other pane, and Space closes it', async () => {
      await clickRow('notes.txt');
      await key(leftList(), { key: ' ' });
      await settle();

      expect(rightPane()?.hidden).toBe(true);
      expect(preview()?.textContent).toContain('hello from portunus');
      expect(api.preview.load).toHaveBeenCalledWith(
        'local',
        '/home/notes.txt',
        expect.objectContaining({ signal: expect.any(AbortSignal) })
      );

      await key(leftList(), { key: ' ' });
      expect(preview()).toBeNull();
      expect(rightPane()?.hidden).toBe(false);
    });

    it('follows the cursor and aborts the load it leaves behind', async () => {
      const pending = deferred<never>();
      vi.mocked(api.files.summarize).mockReturnValue(pending.promise);
      await clickRow('Documents');
      await key(leftList(), { key: ' ' });
      await settle();
      const signal = vi.mocked(api.files.summarize).mock.calls[0]?.[2];
      expect(signal?.aborted).toBe(false);

      await key(leftList(), { key: 'ArrowDown' });
      expect(signal?.aborted).toBe(true);
      await settle();
      expect(api.preview.load).toHaveBeenCalledWith('local', '/home/notes.txt', expect.anything());
      expect(preview()?.textContent).toContain('hello from portunus');
    });

    it('only loads the entry the cursor settles on', async () => {
      await clickRow('Documents');
      await key(leftList(), { key: ' ' });
      await key(leftList(), { key: 'ArrowDown' });
      await settle();
      expect(api.files.summarize).not.toHaveBeenCalled();
      expect(api.preview.load).toHaveBeenCalledTimes(1);
    });

    it('summarizes a folder', async () => {
      await clickRow('Documents');
      await key(leftList(), { key: ' ' });
      await settle();
      expect(preview()?.textContent).toContain('5 items');
      expect(preview()?.textContent).toContain('2 folders, 3 files');
      expect(preview()?.textContent).toContain('2.0 KB in files here');
    });

    it('shows metadata only for a file too large to preview', async () => {
      vi.mocked(api.preview.load).mockResolvedValue({ kind: 'none', reason: 'too_large' });
      await clickRow('notes.txt');
      await key(leftList(), { key: ' ' });
      await settle();
      expect(preview()?.textContent).toContain('File too large to preview (10 B)');
      expect(preview()?.querySelector('dl')?.textContent).toContain('Modified');
    });

    it('Tab moves into the preview and Escape there closes it, keeping the selection', async () => {
      await clickRow('notes.txt');
      await key(leftList(), { key: ' ' });
      await key(leftList(), { key: 'Tab' });
      expect(document.activeElement).toBe(previewBody());

      await key(previewBody(), { key: 'Escape' });
      expect(preview()).toBeNull();
      expect(document.activeElement).toBe(leftList());
      expect(selectedNames()).toEqual(['notes.txt']);
    });

    it('keeps Space for type-ahead right after typed letters', async () => {
      await key(leftList(), { key: 'n' });
      await key(leftList(), { key: ' ' });
      expect(preview()).toBeNull();
    });
  });
});
