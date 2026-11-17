import { act } from 'react';
import { type Root, createRoot } from 'react-dom/client';
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import * as api from '../../api';
import type { Bookmark } from '../../api/types';
import { EMPTY_LISTING } from '../../lib/listing';
import { EMPTY_SELECTION } from '../../lib/selection';
import { useConnections } from '../../stores/connections';
import { usePanes } from '../../stores/panes';
import { entry } from '../../stores/testApi';
import { useUi } from '../../stores/ui';
import { FileList } from './FileList';

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

const ENTRIES = [
  entry('docs', true),
  entry('music', true),
  entry('photos', true),
  entry('notes.txt'),
];

function bookmark(path: string): Bookmark {
  return { id: `id:${path}`, connId: 'local', path };
}

function showList(connId: string | null = 'local', selected: string[] = []) {
  usePanes.setState((s) => ({
    panes: {
      ...s.panes,
      left: {
        ...s.panes.left,
        connId,
        path: '/home',
        listing: { ...EMPTY_LISTING, entries: ENTRIES, done: true },
        selection: { ...EMPTY_SELECTION, selected: new Set(selected) },
      },
    },
  }));
  act(() => root.render(<FileList paneId="left" onEditPath={vi.fn()} />));
}

function rightClick(target: Element) {
  act(() => {
    target.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }));
  });
}

const row = (name: string) =>
  [...host.querySelectorAll('[role=option]')].find((r) => r.textContent?.includes(name))!;
const emptySpace = () => host.querySelector('[role=listbox]')!;
const items = () => [...host.querySelectorAll<HTMLElement>('[role=menuitem]')];
const item = (label: string) => items().find((i) => i.textContent?.includes(label));
const bookmarkItem = () =>
  items().find((i) => /^(Bookmark|Remove bookmark)/.test(i.textContent ?? ''));

beforeEach(() => {
  vi.clearAllMocks();
  usePanes.setState(usePanes.getInitialState(), true);
  useConnections.setState(useConnections.getInitialState(), true);
  useUi.setState(useUi.getInitialState(), true);
  vi.mocked(api.files.list).mockReturnValue({ done: Promise.resolve(), cancel: vi.fn() });
  vi.mocked(api.connections.addBookmark).mockImplementation(async (b) => b);
  vi.mocked(api.connections.saved).mockImplementation(async () => ({
    hosts: [],
    prefs: {},
    bookmarks: [...useConnections.getState().bookmarks],
  }));
  host = document.createElement('div');
  document.body.append(host);
  root = createRoot(host);
});

afterEach(() => {
  act(() => root.unmount());
  host.remove();
});

describe('bookmarking from the file list menu', () => {
  it('bookmarks the current folder from the empty space', async () => {
    showList();
    rightClick(emptySpace());

    expect(bookmarkItem()?.textContent).toBe('Bookmark this folder');
    await act(async () => bookmarkItem()!.click());

    expect(api.connections.addBookmark).toHaveBeenCalledWith({
      id: '',
      connId: 'local',
      path: '/home',
    });
  });

  it('offers to remove the current folder’s bookmark from the empty space', () => {
    useConnections.setState({ bookmarks: [bookmark('/home')] });
    showList();
    rightClick(emptySpace());

    expect(bookmarkItem()?.textContent).toBe('Remove bookmark…');
    act(() => bookmarkItem()!.click());

    expect(useUi.getState().dialog).toEqual({
      kind: 'removeBookmark',
      bookmark: bookmark('/home'),
    });
  });

  it('bookmarks the right-clicked folder', async () => {
    showList();
    rightClick(row('music'));

    expect(bookmarkItem()?.textContent).toBe('Bookmark folder');
    await act(async () => bookmarkItem()!.click());

    expect(api.connections.addBookmark).toHaveBeenCalledTimes(1);
    expect(api.connections.addBookmark).toHaveBeenCalledWith({
      id: '',
      connId: 'local',
      path: '/home/music',
    });
  });

  it('has no bookmark item for a file', () => {
    showList();
    rightClick(row('notes.txt'));

    expect(items().length).toBeGreaterThan(0);
    expect(bookmarkItem()).toBeUndefined();
  });

  it('bookmarks every selected folder that isn’t bookmarked yet', async () => {
    useConnections.setState({ bookmarks: [bookmark('/home/music')] });
    showList('local', ['/home/docs', '/home/music', '/home/photos']);
    rightClick(row('docs'));

    expect(bookmarkItem()?.textContent).toBe('Bookmark 2 folders');
    await act(async () => bookmarkItem()!.click());

    expect(vi.mocked(api.connections.addBookmark).mock.calls.map(([b]) => b.path)).toEqual([
      '/home/docs',
      '/home/photos',
    ]);
  });

  it('offers to remove the bookmark of a bookmarked folder', () => {
    useConnections.setState({ bookmarks: [bookmark('/home/music')] });
    showList();
    rightClick(row('music'));

    expect(bookmarkItem()?.textContent).toBe('Remove bookmark…');
    act(() => bookmarkItem()!.click());

    expect(useUi.getState().dialog).toEqual({
      kind: 'removeBookmark',
      bookmark: bookmark('/home/music'),
    });
    expect(api.connections.addBookmark).not.toHaveBeenCalled();
  });

  it('is disabled with a hint for a connection that isn’t saved', async () => {
    showList('ssh:me@quick.example:22');
    rightClick(row('music'));

    const disabled = item('Bookmark folder') as HTMLButtonElement;
    expect(disabled.disabled).toBe(true);
    expect(disabled.textContent).toContain('Save this host to bookmark its folders');
    await act(async () => disabled.click());
    expect(api.connections.addBookmark).not.toHaveBeenCalled();
  });

  it('is disabled the same way in the empty-space menu', () => {
    showList('ssh:me@quick.example:22');
    rightClick(emptySpace());

    expect((item('Bookmark this folder') as HTMLButtonElement).disabled).toBe(true);
  });
});
