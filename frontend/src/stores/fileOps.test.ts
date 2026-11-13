import { beforeEach, describe, expect, it, vi } from 'vitest';
import * as api from '../api';
import { copyToOtherPane } from './fileOps';
import { usePanes } from './panes';
import { entry } from './testApi';
import { EMPTY_SELECTION } from '../lib/selection';
import { applyPage, finishListing, startListing } from '../lib/listing';

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
