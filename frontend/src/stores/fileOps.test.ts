import { beforeEach, describe, expect, it, vi } from 'vitest';
import * as api from '../api';
import { copyToOtherPane, dropOnPane, moveToOtherPane } from './fileOps';
import { usePanes } from './panes';
import { entry } from './testApi';
import { EMPTY_SELECTION } from '../lib/selection';
import { applyPage, finishListing, startListing } from '../lib/listing';
import { useUi } from './ui';

vi.mock('../api', async () => (await import('./testApi')).createApiMock());

function setPane(id: 'left' | 'right', connId: string, path: string, names: string[]) {
  const entries = names.map((n) => entry(n, false, path));
  let listing = startListing('r', null);
  listing = applyPage(listing, 'r', entries);
  listing = finishListing(listing, 'r');
  usePanes.setState((s) => ({
    panes: {
      ...s.panes,
      [id]: {
        ...s.panes[id],
        connId,
        path,
        listing,
        selection: { ...EMPTY_SELECTION, selected: new Set(entries.map((e) => e.path)) },
      },
    },
  }));
}

beforeEach(() => {
  vi.clearAllMocks();
  usePanes.setState(usePanes.getInitialState(), true);
  useUi.setState(useUi.getInitialState(), true);
});

describe('copyToOtherPane', () => {
  it('does nothing when both panes show the same directory of one connection', () => {
    setPane('left', 'local', '/home', ['a.txt']);
    setPane('right', 'local', '/home', []);
    copyToOtherPane('left');
    expect(api.transfers.copy).not.toHaveBeenCalled();
  });

  it('copies when the directories differ', () => {
    setPane('left', 'local', '/home', ['a.txt']);
    setPane('right', 'local', '/other', []);
    copyToOtherPane('left');
    expect(api.transfers.copy).toHaveBeenCalledWith({
      srcConn: 'local',
      sources: ['/home/a.txt'],
      dstConn: 'local',
      dstDir: '/other',
    });
  });

  it('copies between connections even at the same path', () => {
    setPane('left', 'local', '/home', ['a.txt']);
    setPane('right', 'ssh:h', '/home', []);
    copyToOtherPane('left');
    expect(api.transfers.copy).toHaveBeenCalledTimes(1);
  });
});

describe('moveToOtherPane', () => {
  it('moves the selection into the other pane’s directory', () => {
    setPane('left', 'local', '/home', ['a.txt', 'b.txt']);
    setPane('right', 'ssh:h', '/srv', []);
    moveToOtherPane('left');
    expect(api.transfers.move).toHaveBeenCalledWith({
      srcConn: 'local',
      sources: ['/home/a.txt', '/home/b.txt'],
      dstConn: 'ssh:h',
      dstDir: '/srv',
    });
    expect(api.transfers.copy).not.toHaveBeenCalled();
  });

  it('does nothing when both panes show the same directory of one connection', () => {
    setPane('left', 'local', '/home', ['a.txt']);
    setPane('right', 'local', '/home', []);
    moveToOtherPane('left');
    expect(api.transfers.move).not.toHaveBeenCalled();
  });

  it('shows the failure when the move cannot start', async () => {
    setPane('left', 'local', '/home', ['a.txt']);
    setPane('right', 'local', '/other', []);
    vi.mocked(api.transfers.move).mockRejectedValue(new Error('nope'));
    moveToOtherPane('left');
    await vi.waitFor(() => expect(useUi.getState().dialog).toMatchObject({ kind: 'error' }));
  });
});

describe('dropOnPane', () => {
  const payload = { connId: 'local', paths: ['/home/a.txt'] };

  it('copies by default', () => {
    setPane('right', 'ssh:h', '/srv', []);
    dropOnPane('right', payload, '/srv');
    expect(api.transfers.copy).toHaveBeenCalledWith({
      srcConn: 'local',
      sources: ['/home/a.txt'],
      dstConn: 'ssh:h',
      dstDir: '/srv',
    });
    expect(api.transfers.move).not.toHaveBeenCalled();
  });

  it('moves when asked to, into a folder of the pane', () => {
    setPane('right', 'ssh:h', '/srv', []);
    dropOnPane('right', payload, '/srv/folder', 'move');
    expect(api.transfers.move).toHaveBeenCalledWith({
      srcConn: 'local',
      sources: ['/home/a.txt'],
      dstConn: 'ssh:h',
      dstDir: '/srv/folder',
    });
    expect(api.transfers.copy).not.toHaveBeenCalled();
  });

  it('ignores a move into the directory the entries already live in', () => {
    setPane('left', 'local', '/home', ['a.txt']);
    dropOnPane('left', payload, '/home', 'move');
    expect(api.transfers.move).not.toHaveBeenCalled();
  });
});
