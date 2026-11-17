import { act } from 'react';
import { type Root, createRoot } from 'react-dom/client';
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import * as api from '../../api';
import { useConnections } from '../../stores/connections';
import { usePanes } from '../../stores/panes';
import { bindingError, localConnection } from '../../stores/testApi';
import { useUi } from '../../stores/ui';
import { QuickConnectDialog } from './QuickConnectDialog';

vi.mock('../../api', async () => (await import('../../stores/testApi')).createApiMock());

let root: Root;
let host: HTMLElement;
const onClose = vi.fn();

function showModal(this: HTMLDialogElement) {
  this.setAttribute('open', '');
}

function close(this: HTMLDialogElement) {
  this.removeAttribute('open');
}

beforeAll(() => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
  HTMLDialogElement.prototype.showModal ??= showModal;
  HTMLDialogElement.prototype.close ??= close;
});

beforeEach(() => {
  vi.clearAllMocks();
  usePanes.setState(usePanes.getInitialState(), true);
  useConnections.setState(useConnections.getInitialState(), true);
  useUi.setState(useUi.getInitialState(), true);
  vi.mocked(api.files.list).mockReturnValue({ done: Promise.resolve(), cancel: vi.fn() });
  vi.mocked(api.connections.saveSSHHost).mockImplementation(async (h) => h);
  host = document.createElement('div');
  document.body.append(host);
  root = createRoot(host);
  act(() => root.render(<QuickConnectDialog paneId="left" onClose={onClose} />));
});

afterEach(() => {
  act(() => root.unmount());
  host.remove();
});

function type(input: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!;
  act(() => {
    setter.call(input, value);
    input.dispatchEvent(new Event('input', { bubbles: true }));
  });
}

const field = (label: string) =>
  [...host.querySelectorAll('label')]
    .find((l) => l.textContent?.startsWith(label))!
    .querySelector('input')!;

async function submit() {
  await act(async () => {
    host
      .querySelector('form')!
      .dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
  });
}

describe('QuickConnectDialog save as a host', () => {
  it('saves the host under the chosen name once the connection succeeds', async () => {
    vi.mocked(api.connections.connect).mockResolvedValue(localConnection());
    type(field('Server'), 'me@box:2222');
    act(() => field('Save as a host').click());
    expect(field('Name').value).toBe('me@box:2222');
    type(field('Name'), 'Box');

    await submit();

    expect(onClose).toHaveBeenCalled();
    expect(api.connections.saveSSHHost).toHaveBeenCalledWith({
      id: '',
      name: 'Box',
      user: 'me',
      host: 'box',
      port: 2222,
      connId: '',
    });
  });

  it('does not save when the connection fails', async () => {
    vi.mocked(api.connections.connect).mockRejectedValue(bindingError('other', 'refused'));
    type(field('Server'), 'me@box');
    act(() => field('Save as a host').click());

    await submit();

    expect(api.connections.connect).toHaveBeenCalled();
    expect(api.connections.saveSSHHost).not.toHaveBeenCalled();
  });

  it('connects without saving when the box is unchecked', async () => {
    vi.mocked(api.connections.connect).mockResolvedValue(localConnection());
    type(field('Server'), 'me@box');

    await submit();

    expect(api.connections.connect).toHaveBeenCalled();
    expect(api.connections.saveSSHHost).not.toHaveBeenCalled();
  });

  it('reports a failed save', async () => {
    vi.mocked(api.connections.connect).mockResolvedValue(localConnection());
    vi.mocked(api.connections.saveSSHHost).mockRejectedValue(bindingError('other', 'disk full'));
    type(field('Server'), 'me@box');
    act(() => field('Save as a host').click());

    await submit();

    expect(useUi.getState().dialog).toMatchObject({ kind: 'error', message: 'disk full' });
  });
});
