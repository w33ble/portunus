import { act } from 'react';
import { type Root, createRoot } from 'react-dom/client';
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import * as api from './api';
import { JobKind, State } from './api/enums';
import type { JobView, Update } from './api/types';
import App from './App';
import { usePanes } from './stores/panes';
import { usePrompts } from './stores/prompts';
import { useSettings } from './stores/settings';
import { useTransfers } from './stores/transfers';
import { entry, localConnection } from './stores/testApi';
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
    filesPage: on('filesPage'),
    transfersUpdate: on('transfersUpdate'),
    connectionsChanged: on('connectionsChanged'),
    dropFiles: on('dropFiles'),
    promptHostKey: on('promptHostKey'),
    promptSecret: on('promptSecret'),
    promptConflict: on('promptConflict'),
    promptResolved: on('promptResolved'),
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
  vi.mocked(api.connections.list).mockResolvedValue([localConnection()]);
  vi.mocked(api.connections.sshHosts).mockResolvedValue([{ alias: 'prod', source: 'c:1' }]);
  vi.mocked(api.connections.sshAvailable).mockResolvedValue('');
  vi.mocked(api.connections.secretsSessionOnly).mockResolvedValue(false);
  vi.mocked(api.connections.s3Profiles).mockResolvedValue([]);
  vi.mocked(api.connections.connect).mockResolvedValue(localConnection());
  vi.mocked(api.transfers.jobs).mockResolvedValue([]);
  vi.mocked(api.prompts.pending).mockResolvedValue({ hostKeys: [], secrets: [], conflicts: [] });
  vi.mocked(api.files.list).mockImplementation((_conn, _dir, requestId) => {
    queueMicrotask(() =>
      handlers.get('filesPage')?.({
        requestId,
        entries: [entry('Documents', true), entry('notes.txt'), entry('.hidden')],
      } as never)
    );
    return { total: Promise.resolve(3), cancel: vi.fn() };
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

describe('App', () => {
  it('opens both panes on the local home and hides dotfiles', () => {
    expect(rowNames('left')[0]).toContain('Documents');
    expect(rowNames('left').some((t) => t.includes('notes.txt'))).toBe(true);
    expect(rowNames('left').some((t) => t.includes('.hidden'))).toBe(false);
    expect(rowNames('right')).toHaveLength(2);
    expect(host.querySelectorAll('[data-file-drop-target]')).toHaveLength(2);
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
});
