import { act } from 'react';
import { type Root, createRoot } from 'react-dom/client';
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { Kind, TargetKind } from '../../api/enums';
import { useConnections } from '../../stores/connections';
import { usePanes } from '../../stores/panes';
import { useUi } from '../../stores/ui';
import { ConnectionPicker } from './ConnectionPicker';

vi.mock('../../api', async () => (await import('../../stores/testApi')).createApiMock());

let root: Root;
let host: HTMLElement;

beforeAll(() => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
});

beforeEach(() => {
  usePanes.setState(usePanes.getInitialState(), true);
  useConnections.setState(useConnections.getInitialState(), true);
  useUi.setState(useUi.getInitialState(), true);
  host = document.createElement('div');
  document.body.append(host);
  root = createRoot(host);
});

afterEach(() => {
  act(() => root.unmount());
  host.remove();
});

function renderPicker() {
  act(() => root.render(<ConnectionPicker paneId="left" />));
  return host.querySelector('select')!;
}

describe('ConnectionPicker', () => {
  it('shows an option for a connection that is in no list and has been dropped', () => {
    usePanes.setState((s) => ({
      panes: {
        ...s.panes,
        left: {
          ...s.panes.left,
          connId: 's3:gone',
          lastTarget: {
            target: { kind: TargetKind.TargetS3, profileId: 'gone' },
            label: 'Archive',
          },
        },
      },
    }));

    const select = renderPicker();
    expect(select.value).toBe('s3:gone');
    expect([...select.options].find((o) => o.value === 's3:gone')?.textContent).toBe('Archive');
  });

  it('labels a live quick connection from the connection list', () => {
    useConnections.setState({
      connections: [
        {
          id: 'ssh:me@h:22',
          kind: Kind.KindSFTP,
          label: 'me@h',
          home: '/',
          target: { kind: TargetKind.TargetSSH, host: 'h', user: 'me', port: 22 },
        },
      ],
    });
    usePanes.setState((s) => ({
      panes: { ...s.panes, left: { ...s.panes.left, connId: 'ssh:me@h:22' } },
    }));

    const select = renderPicker();
    expect(select.value).toBe('ssh:me@h:22');
    expect(select.selectedOptions[0]?.textContent).toBe('me@h');
  });

  it('selects the listed profile when the pane uses it', () => {
    useConnections.setState({ profiles: [{ id: 'p1', name: 'Prod bucket' } as never] });
    usePanes.setState((s) => ({
      panes: { ...s.panes, left: { ...s.panes.left, connId: 's3:p1' } },
    }));

    expect(renderPicker().selectedOptions[0]?.textContent).toBe('Prod bucket');
  });

  it('lists SSH hosts and S3 profiles in case-insensitive natural order', () => {
    useConnections.setState({
      hosts: ['web10', 'Zeta', 'alpha', 'web2', 'Beta'].map((alias) => ({ alias, source: 'c:1' })),
      profiles: [
        { id: 'p1', name: 'logs' },
        { id: 'p2', name: 'Archive' },
      ] as never,
    });

    const select = renderPicker();
    const group = (label: string) =>
      [...select.querySelectorAll(`optgroup[label="${label}"] option`)].map((o) => o.textContent);
    expect(group('SSH hosts')).toEqual(['alpha', 'Beta', 'web2', 'web10', 'Zeta']);
    expect(group('S3')).toEqual(['Archive', 'logs']);
  });

  it('lists saved hosts with config aliases and leaves out hidden connections', () => {
    useConnections.setState({
      hosts: [{ alias: 'web', source: 'c:1' }],
      savedHosts: [
        { id: 'h1', name: 'Alpha box', host: 'a.example', connId: 'ssh:a.example' },
        { id: 'h2', name: 'Secret box', host: 's.example', connId: 'ssh:s.example' },
      ],
      profiles: [{ id: 'p1', name: 'Quiet bucket' }] as never,
      prefs: {
        'ssh:s.example': { hideInPicker: true },
        's3:p1': { hideInPicker: true },
      },
    });

    const select = renderPicker();
    const group = (label: string) =>
      [...select.querySelectorAll(`optgroup[label="${label}"] option`)].map((o) => o.textContent);
    expect(group('SSH hosts')).toEqual(['Alpha box', 'web']);
    expect(select.querySelector('optgroup[label="S3"]')).toBeNull();
  });

  it('keeps a hidden connection selectable while the pane is on it', () => {
    useConnections.setState({
      savedHosts: [{ id: 'h2', name: 'Secret box', host: 's.example', connId: 'ssh:s.example' }],
      prefs: { 'ssh:s.example': { hideInPicker: true } },
    });
    usePanes.setState((s) => ({
      panes: { ...s.panes, left: { ...s.panes.left, connId: 'ssh:s.example' } },
    }));

    expect(renderPicker().selectedOptions[0]?.textContent).toBe('Secret box');
  });

  it('connects to a chosen saved host', async () => {
    const connect = vi.spyOn(usePanes.getState(), 'connectTo').mockResolvedValue(true);
    useConnections.setState({
      savedHosts: [
        { id: 'h1', name: 'Alpha box', user: 'me', host: 'a.example', connId: 'ssh:me@a.example' },
      ],
    });
    const select = renderPicker();

    act(() => choose(select, 'ssh:me@a.example'));

    expect(connect).toHaveBeenCalledWith(
      'left',
      { kind: 'ssh', user: 'me', host: 'a.example' },
      'Alpha box'
    );
  });

  it('opens the connections dialog on the pane’s connection', () => {
    useConnections.setState({ profiles: [{ id: 'p1', name: 'Prod' }] as never });
    usePanes.setState((s) => ({
      panes: { ...s.panes, left: { ...s.panes.left, connId: 's3:p1' } },
    }));
    const select = renderPicker();

    act(() => choose(select, '__manage'));

    expect(useUi.getState().dialog).toEqual({ kind: 'connections', connId: 's3:p1' });
  });
});

function choose(select: HTMLSelectElement, value: string) {
  select.value = value;
  select.dispatchEvent(new Event('change', { bubbles: true }));
}
