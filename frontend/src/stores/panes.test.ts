import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import * as api from '../api';
import { shownEntries } from '../lib/listing';
import { useConnections } from './connections';
import { receivePage, usePanes } from './panes';
import { bindingError, deferred, entry, localConnection } from './testApi';
import { useUi } from './ui';

vi.mock('../api', async () => (await import('./testApi')).createApiMock());

const listMock = vi.mocked(api.files.list);
const connectMock = vi.mocked(api.connections.connect);

interface FakeList {
  requestId: string;
  total: ReturnType<typeof deferred<number>>;
  cancel: ReturnType<typeof vi.fn>;
}

/** Each List call gets a controllable handle so tests decide when the total resolves. */
function captureLists(): FakeList[] {
  const lists: FakeList[] = [];
  listMock.mockImplementation((_conn, _dir, requestId) => {
    const total = deferred<number>();
    const cancel = vi.fn();
    lists.push({ requestId, total, cancel });
    return { total: total.promise, cancel };
  });
  return lists;
}

const left = () => usePanes.getState().panes.left;
const names = () => shownEntries(left().listing).map((e) => e.name);

async function openLocal() {
  connectMock.mockResolvedValue(localConnection());
  await usePanes.getState().connectTo('left', api.targets.local(), 'Local');
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.clearAllMocks();
  usePanes.setState(usePanes.getInitialState(), true);
  useConnections.setState(useConnections.getInitialState(), true);
  useUi.setState(useUi.getInitialState(), true);
});

afterEach(() => vi.useRealTimers());

describe('connecting', () => {
  it('lists the connection home after connecting', async () => {
    const lists = captureLists();
    await openLocal();
    expect(left()).toMatchObject({ connId: 'local', path: '/home', status: { kind: 'loading' } });
    expect(lists).toHaveLength(1);
    expect(useConnections.getState().connections.map((c) => c.id)).toEqual(['local']);
  });

  it('shows connecting while the dial is in flight', () => {
    captureLists();
    const dial = deferred<ReturnType<typeof localConnection>>();
    connectMock.mockReturnValue(dial.promise);
    void usePanes.getState().connectTo('left', api.targets.ssh('prod'), 'prod');
    expect(left().status).toEqual({ kind: 'connecting', label: 'prod' });
  });

  it('keeps the old listing and reports the error when a connect fails', async () => {
    captureLists();
    await openLocal();
    connectMock.mockRejectedValue(bindingError('other', 'connection refused'));
    await usePanes.getState().connectTo('left', api.targets.ssh('down'), 'down');
    expect(left().connId).toBe('local');
    expect(left().status).toMatchObject({
      kind: 'error',
      error: { message: 'connection refused' },
    });
    expect(useUi.getState().dialog).toMatchObject({
      kind: 'error',
      title: 'Couldn’t connect to down',
    });
  });

  it('explains a host key mismatch', async () => {
    connectMock.mockRejectedValue(
      bindingError('hostkey_mismatch', 'key for h changed (known_hosts:3)')
    );
    await usePanes.getState().connectTo('right', api.targets.ssh('h'), 'h');
    const { dialog } = useUi.getState();
    expect(dialog).toMatchObject({ kind: 'error', title: 'Host key changed' });
    expect(dialog?.kind === 'error' && dialog.hint).toMatch(/known_hosts/);
  });

  it('ignores a connect that was superseded', async () => {
    captureLists();
    const slow = deferred<ReturnType<typeof localConnection>>();
    connectMock.mockReturnValueOnce(slow.promise);
    const first = usePanes.getState().connectTo('left', api.targets.ssh('slow'), 'slow');
    connectMock.mockResolvedValueOnce(localConnection());
    await usePanes.getState().connectTo('left', api.targets.local(), 'Local');
    slow.resolve({ ...localConnection(), id: 'ssh:slow', home: '/slow' });
    await first;
    expect(left().connId).toBe('local');
  });
});

describe('streaming listings', () => {
  it('becomes ready when pages arrive before the total', async () => {
    const lists = captureLists();
    await openLocal();
    const [list] = lists;
    receivePage(list!.requestId, [entry('a'), entry('b')]);
    vi.advanceTimersByTime(100);
    expect(names()).toEqual(['a', 'b']);
    expect(left().status.kind).toBe('loading');

    list!.total.resolve(2);
    await vi.advanceTimersByTimeAsync(0);
    expect(left().status.kind).toBe('ready');
  });

  it('becomes ready when the total resolves before the last page', async () => {
    const lists = captureLists();
    await openLocal();
    const [list] = lists;
    receivePage(list!.requestId, [entry('a')]);
    vi.advanceTimersByTime(100);

    list!.total.resolve(3);
    await vi.advanceTimersByTimeAsync(0);
    expect(left().status.kind).toBe('loading');

    receivePage(list!.requestId, [entry('b'), entry('c')]);
    vi.advanceTimersByTime(100);
    expect(left().status.kind).toBe('ready');
    expect(names()).toEqual(['a', 'b', 'c']);
  });

  it('flushes buffered pages when the total arrives', async () => {
    const lists = captureLists();
    await openLocal();
    const [list] = lists;
    receivePage(list!.requestId, [entry('a')]);
    list!.total.resolve(1);
    await vi.advanceTimersByTimeAsync(0);
    expect(left().status.kind).toBe('ready');
  });

  it('treats an empty directory as ready', async () => {
    const lists = captureLists();
    await openLocal();
    lists[0]!.total.resolve(0);
    await vi.advanceTimersByTimeAsync(0);
    expect(left().status.kind).toBe('ready');
    expect(names()).toEqual([]);
  });

  it('cancels the previous listing and drops its late pages on navigate', async () => {
    const lists = captureLists();
    await openLocal();
    usePanes.getState().navigate('left', '/home/docs');
    expect(lists[0]!.cancel).toHaveBeenCalled();
    expect(left().path).toBe('/home/docs');

    receivePage(lists[0]!.requestId, [entry('stale')]);
    receivePage(lists[1]!.requestId, [entry('fresh', false, '/home/docs')]);
    vi.advanceTimersByTime(100);
    expect(names()).toEqual(['fresh']);
  });

  it('keeps the old entries on screen while a refresh streams in', async () => {
    const lists = captureLists();
    await openLocal();
    receivePage(lists[0]!.requestId, [entry('old')]);
    vi.advanceTimersByTime(100);
    lists[0]!.total.resolve(1);
    await vi.advanceTimersByTimeAsync(0);

    usePanes.getState().refresh('left');
    expect(names()).toEqual(['old']);
    receivePage(lists[1]!.requestId, [entry('new')]);
    vi.advanceTimersByTime(100);
    expect(names()).toEqual(['old']);
    lists[1]!.total.resolve(1);
    await vi.advanceTimersByTimeAsync(0);
    expect(names()).toEqual(['new']);
  });

  it('goes to the error state when List fails, and ignores cancellation', async () => {
    const lists = captureLists();
    await openLocal();
    lists[0]!.total.reject(bindingError('permission', 'open /root: permission denied'));
    await vi.advanceTimersByTimeAsync(0);
    expect(left().status).toMatchObject({ kind: 'error', error: { kind: 'permission' } });

    usePanes.getState().refresh('left');
    const cancelled = new Error('Promise cancelled.');
    cancelled.name = 'CancelError';
    lists[1]!.total.reject(cancelled);
    await vi.advanceTimersByTimeAsync(0);
    expect(left().status.kind).toBe('loading');
  });
});

describe('refreshWhere', () => {
  it('refreshes only panes showing the directory', async () => {
    const lists = captureLists();
    await openLocal();
    usePanes.getState().refreshWhere('local', '/elsewhere');
    expect(lists).toHaveLength(1);
    usePanes.getState().refreshWhere('local', '/home');
    expect(lists).toHaveLength(2);
    usePanes.getState().refreshWhere('other-conn');
    expect(lists).toHaveLength(2);
  });
});

describe('navigation helpers', () => {
  it('goUp stops at the root', async () => {
    captureLists();
    await openLocal();
    usePanes.getState().goUp('left');
    expect(left().path).toBe('/');
    usePanes.getState().goUp('left');
    expect(listMock).toHaveBeenCalledTimes(2);
  });

  it('sort toggling is per pane', () => {
    usePanes.getState().toggleSort('left', 'size');
    expect(left().sort).toEqual({ key: 'size', dir: 'asc' });
    expect(usePanes.getState().panes.right.sort).toEqual({ key: 'name', dir: 'asc' });
  });
});

describe('reconnecting', () => {
  it('redials the last target and relists the current directory', async () => {
    const lists = captureLists();
    connectMock.mockResolvedValue({ ...localConnection(), id: 's3:p', home: '/' });
    const target = api.targets.s3('p');
    await usePanes.getState().connectTo('left', target, 'Prod');
    usePanes.getState().navigate('left', '/bucket/dir');
    expect(left().lastTarget).toEqual({ target, label: 'Prod' });

    connectMock.mockClear();
    await usePanes.getState().reconnect('left');
    expect(connectMock).toHaveBeenCalledWith(target);
    expect(left()).toMatchObject({ connId: 's3:p', path: '/bucket/dir' });
    expect(lists.at(-1)).toBeDefined();
  });

  it('does nothing for a pane that never connected', async () => {
    await usePanes.getState().reconnect('right');
    expect(connectMock).not.toHaveBeenCalled();
  });
});
