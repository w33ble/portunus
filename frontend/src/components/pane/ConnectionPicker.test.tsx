import { act } from 'react';
import { type Root, createRoot } from 'react-dom/client';
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { Kind, TargetKind } from '../../api/enums';
import { useConnections } from '../../stores/connections';
import { usePanes } from '../../stores/panes';
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
});
