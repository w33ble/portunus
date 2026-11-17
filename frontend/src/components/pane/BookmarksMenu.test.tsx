import { act } from 'react';
import { type Root, createRoot } from 'react-dom/client';
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import * as api from '../../api';
import type { Bookmark } from '../../api/types';
import { IS_MAC } from '../../lib/platform';
import { useConnections } from '../../stores/connections';
import { usePanes } from '../../stores/panes';
import { useRecents } from '../../stores/recents';
import { localConnection } from '../../stores/testApi';
import { useUi } from '../../stores/ui';
import { DialogHost } from '../dialogs/DialogHost';
import { BookmarksMenu } from './BookmarksMenu';
import { Pane } from './Pane';

vi.mock('../../api', async () => (await import('../../stores/testApi')).createApiMock());

let root: Root;
let host: HTMLElement;

beforeAll(() => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
  Object.defineProperty(HTMLElement.prototype, 'offsetHeight', { configurable: true, value: 600 });
  Object.defineProperty(HTMLElement.prototype, 'offsetWidth', { configurable: true, value: 600 });
  Object.defineProperty(HTMLElement.prototype, 'clientHeight', { configurable: true, value: 600 });
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
});

beforeEach(() => {
  vi.clearAllMocks();
  usePanes.setState(usePanes.getInitialState(), true);
  useConnections.setState(useConnections.getInitialState(), true);
  useRecents.setState(useRecents.getInitialState(), true);
  useUi.setState(useUi.getInitialState(), true);
  vi.mocked(api.files.list).mockReturnValue({ done: Promise.resolve(), cancel: vi.fn() });
  host = document.createElement('div');
  document.body.append(host);
  root = createRoot(host);
});

afterEach(() => {
  act(() => root.unmount());
  host.remove();
});

function showPane(connId: string, path: string) {
  usePanes.setState((s) => ({
    panes: { ...s.panes, left: { ...s.panes.left, connId, path } },
  }));
}

function bookmark(id: string, connId: string, path: string, extra: Partial<Bookmark> = {}) {
  return { id, connId, path, ...extra };
}

function openMenu() {
  act(() => root.render(<BookmarksMenu paneId="left" />));
  act(() => host.querySelector<HTMLElement>('[aria-label="Bookmarks"]')!.click());
}

const items = () => [...host.querySelectorAll<HTMLButtonElement>('[role=menuitem]')];
const item = (text: string) => items().find((i) => i.textContent?.includes(text))!;
const headings = () => [...host.querySelectorAll('[role=presentation]')].map((h) => h.textContent);

describe('BookmarksMenu', () => {
  it('lists favorites, this connection’s bookmarks and recent folders without the current one', () => {
    useConnections.setState({
      profiles: [{ id: 'p1', name: 'Prod' } as never],
      bookmarks: [
        bookmark('b1', 's3:p1', '/logs', { pinned: true }),
        bookmark('b2', 'local', '/home/docs'),
        bookmark('b3', 'local', '/home/work', { name: 'Work' }),
      ],
    });
    useRecents.setState({ byConn: { local: ['/home', '/tmp', '/var'] } });
    showPane('local', '/home');

    openMenu();

    expect(headings()).toEqual(['Favorites', 'Bookmarks on Local', 'Recent folders']);
    expect(items().map((i) => i.textContent)).toEqual([
      `Bookmark this folder${IS_MAC ? '⌘D' : 'Ctrl+D'}`,
      'logsProd',
      'docs/home/docs',
      'Work/home/work',
      '/tmp',
      '/var',
      'Manage bookmarks…',
    ]);
  });

  it('offers to remove the bookmark on the current folder', () => {
    useConnections.setState({ bookmarks: [bookmark('b2', 'local', '/home/docs')] });
    showPane('local', '/home/docs');

    openMenu();

    expect(item('Remove bookmark…')).toBeDefined();
  });

  it('connects to another connection and opens a favorite’s folder', async () => {
    useConnections.setState({
      profiles: [{ id: 'p1', name: 'Prod' } as never],
      bookmarks: [bookmark('b1', 's3:p1', '/logs', { pinned: true })],
    });
    showPane('local', '/home');
    vi.mocked(api.connections.connect).mockResolvedValue({
      ...localConnection(),
      id: 's3:p1',
      label: 'Prod',
      home: '/',
    });
    openMenu();

    await act(async () => item('logs').click());

    expect(api.connections.connect).toHaveBeenCalledWith({ kind: 's3', profileId: 'p1' });
    expect(usePanes.getState().panes.left).toMatchObject({ connId: 's3:p1', path: '/logs' });
  });

  it('navigates to a recent folder', () => {
    useRecents.setState({ byConn: { local: ['/home', '/tmp'] } });
    showPane('local', '/home');
    openMenu();

    act(() => item('/tmp').click());

    expect(usePanes.getState().panes.left.path).toBe('/tmp');
    expect(host.querySelector('[role=menu]')).toBeNull();
  });

  it('cannot bookmark a connection that is not saved', () => {
    showPane('ssh:me@h:22', '/srv');

    openMenu();

    expect(item('Bookmark this folder').disabled).toBe(true);
    expect(item('Bookmark this folder').textContent).toContain(
      'Save this host to bookmark its folders'
    );
  });

  it('opens the connections dialog on this connection from Manage bookmarks', () => {
    showPane('local', '/home');
    openMenu();

    act(() => item('Manage bookmarks…').click());

    expect(useUi.getState().dialog).toEqual({ kind: 'connections', connId: 'local' });
  });

  it('closes on Escape', () => {
    showPane('local', '/home');
    openMenu();

    act(() => {
      host
        .querySelector('[role=menu]')!
        .dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
    });

    expect(host.querySelector('[role=menu]')).toBeNull();
  });
});

const press = (pane: HTMLElement) =>
  act(async () => {
    pane.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'd', ctrlKey: true, bubbles: true, cancelable: true })
    );
  });

const button = (name: string) =>
  [...host.querySelectorAll<HTMLButtonElement>('dialog button')].find(
    (b) => b.textContent === name
  )!;

describe('bookmark shortcut', () => {
  it('Ctrl+D bookmarks the folder', async () => {
    const added = bookmark('b9', 'local', '/home');
    vi.mocked(api.connections.addBookmark).mockResolvedValue(added);
    vi.mocked(api.connections.saved).mockResolvedValue({
      hosts: [],
      prefs: {},
      bookmarks: [added],
    });
    showPane('local', '/home');
    await act(async () => root.render(<Pane paneId="left" />));

    await press(host.querySelector('section')!);

    expect(api.connections.addBookmark).toHaveBeenCalledWith({
      id: '',
      connId: 'local',
      path: '/home',
    });
    expect(useConnections.getState().bookmarks).toEqual([added]);
  });

  it('Ctrl+D on a bookmarked folder asks first, and only Remove deletes it', async () => {
    const existing = bookmark('b9', 'local', '/home', { name: 'Home base' });
    useConnections.setState({ bookmarks: [existing] });
    vi.mocked(api.connections.removeBookmark).mockResolvedValue(undefined);
    vi.mocked(api.connections.saved).mockResolvedValue({ hosts: [], prefs: {}, bookmarks: [] });
    showPane('local', '/home');
    await act(async () =>
      root.render(
        <>
          <Pane paneId="left" />
          <DialogHost />
        </>
      )
    );

    await press(host.querySelector('section')!);
    expect(host.querySelector('dialog')?.textContent).toContain('Remove bookmark?');
    expect(host.querySelector('dialog')?.textContent).toContain('Home base');
    expect(host.querySelector('dialog')?.textContent).toContain('/home');
    expect(api.connections.removeBookmark).not.toHaveBeenCalled();

    await act(async () => button('Cancel').click());
    expect(host.querySelector('dialog')).toBeNull();
    expect(api.connections.removeBookmark).not.toHaveBeenCalled();

    await press(host.querySelector('section')!);
    await act(async () => button('Remove').click());
    expect(api.connections.removeBookmark).toHaveBeenCalledWith('b9');
    expect(useConnections.getState().bookmarks).toEqual([]);
    expect(host.querySelector('dialog')).toBeNull();
  });

  it('shows a backend error in the dialog and keeps it open', async () => {
    useConnections.setState({ bookmarks: [bookmark('b9', 'local', '/home')] });
    vi.mocked(api.connections.removeBookmark).mockRejectedValue(new Error('disk full'));
    showPane('local', '/home');
    await act(async () =>
      root.render(
        <>
          <Pane paneId="left" />
          <DialogHost />
        </>
      )
    );

    await press(host.querySelector('section')!);
    await act(async () => button('Remove').click());

    expect(host.querySelector('dialog')?.textContent).toContain('disk full');
  });

  it('the menu item opens the same confirmation', async () => {
    useConnections.setState({ bookmarks: [bookmark('b2', 'local', '/home/docs')] });
    showPane('local', '/home/docs');
    act(() =>
      root.render(
        <>
          <BookmarksMenu paneId="left" />
          <DialogHost />
        </>
      )
    );
    act(() => host.querySelector<HTMLElement>('[aria-label="Bookmarks"]')!.click());

    await act(async () => item('Remove bookmark…').click());

    expect(host.querySelector('dialog')?.textContent).toContain('Remove bookmark?');
    expect(api.connections.removeBookmark).not.toHaveBeenCalled();
  });
});
