import { beforeEach, describe, expect, it, vi } from 'vitest';
import * as api from '../api';
import { JobKind, State } from '../api/enums';
import type { JobView } from '../api/types';
import { usePanes } from './panes';
import { useTransfers } from './transfers';
import { useUi } from './ui';

vi.mock('../api', async () => (await import('./testApi')).createApiMock());

function job(id: string, over: Partial<JobView> = {}): JobView {
  return {
    id,
    kind: JobKind.KindCopy,
    state: State.StateRunning,
    srcConn: 'local',
    dstConn: 'ssh:h',
    dstDir: '/srv',
    title: id,
    walking: false,
    totalFiles: 1,
    doneFiles: 0,
    failedFiles: 0,
    skippedFiles: 0,
    totalBytes: 10,
    doneBytes: 0,
    bytesPerSec: 0,
    etaSeconds: -1,
    active: null,
    failed: null,
    createdAt: '2026-01-01T00:00:00Z',
    ...over,
  };
}

const refreshWhere = vi.fn();

beforeEach(() => {
  vi.clearAllMocks();
  useTransfers.setState(useTransfers.getInitialState(), true);
  useUi.setState(useUi.getInitialState(), true);
  usePanes.setState({ refreshWhere });
});

describe('useTransfers', () => {
  it('seeds from Jobs() then applies updates', async () => {
    vi.mocked(api.transfers.jobs).mockResolvedValue([job('a'), job('b')]);
    await useTransfers.getState().init();
    useTransfers.getState().apply({ jobs: [job('b', { doneBytes: 5 }), job('c')], removed: ['a'] });
    const jobs = useTransfers.getState().jobs;
    expect(Object.keys(jobs).toSorted()).toEqual(['b', 'c']);
    expect(jobs['b']?.doneBytes).toBe(5);
  });

  it('keeps updates that arrived before the snapshot', async () => {
    useTransfers.getState().apply({ jobs: [job('a', { doneBytes: 9 })], removed: null });
    vi.mocked(api.transfers.jobs).mockResolvedValue([job('a', { doneBytes: 1 }), job('b')]);
    await useTransfers.getState().init();
    expect(useTransfers.getState().jobs['a']?.doneBytes).toBe(9);
    expect(useTransfers.getState().jobs['b']).toBeDefined();
  });

  it('refreshes the destination pane once when a copy finishes', () => {
    useTransfers.getState().apply({ jobs: [job('a')], removed: null });
    expect(refreshWhere).not.toHaveBeenCalled();
    useTransfers.getState().apply({ jobs: [job('a', { state: State.StateDone })], removed: null });
    expect(refreshWhere).toHaveBeenCalledWith('ssh:h', '/srv');
    useTransfers
      .getState()
      .apply({ jobs: [job('a', { state: State.StateDone, doneBytes: 10 })], removed: null });
    expect(refreshWhere).toHaveBeenCalledTimes(1);
  });

  it('refreshes both ends when a move finishes', () => {
    const move = { kind: JobKind.KindMove, srcConn: 'local', dstConn: 'ssh:h', dstDir: '/srv' };
    useTransfers.getState().apply({ jobs: [job('m', move)], removed: null });
    useTransfers
      .getState()
      .apply({ jobs: [job('m', { ...move, state: State.StateDone })], removed: null });
    expect(refreshWhere).toHaveBeenCalledWith('local');
    expect(refreshWhere).toHaveBeenCalledWith('ssh:h', '/srv');
  });

  it('refreshes the connection once when a move stays on it', () => {
    const move = { kind: JobKind.KindMove, srcConn: 'local', dstConn: 'local', dstDir: '/srv' };
    useTransfers.getState().apply({ jobs: [job('m', move)], removed: null });
    useTransfers
      .getState()
      .apply({ jobs: [job('m', { ...move, state: State.StateCanceled })], removed: null });
    expect(refreshWhere.mock.calls).toEqual([['local']]);
  });

  it('refreshes the source connection when a delete finishes', () => {
    useTransfers.getState().apply({
      jobs: [job('d', { kind: JobKind.KindDelete, srcConn: 'local', dstConn: '' })],
      removed: null,
    });
    useTransfers.getState().apply({
      jobs: [
        job('d', {
          kind: JobKind.KindDelete,
          srcConn: 'local',
          dstConn: '',
          state: State.StateDone,
        }),
      ],
      removed: null,
    });
    expect(refreshWhere).toHaveBeenCalledWith('local');
  });

  it('reports failures to resume through the error dialog', async () => {
    vi.mocked(api.transfers.resume).mockRejectedValue(new Error('gone'));
    await useTransfers.getState().resume('a');
    expect(useUi.getState().dialog).toMatchObject({ kind: 'error' });
  });

  it('reports failures to cancel through the error dialog', async () => {
    vi.mocked(api.transfers.cancel).mockRejectedValue(new Error('gone'));
    await useTransfers.getState().cancel('a');
    expect(useUi.getState().dialog).toMatchObject({ kind: 'error' });
  });
});
