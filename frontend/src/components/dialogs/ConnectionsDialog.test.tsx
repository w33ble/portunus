import { act } from 'react';
import { type Root, createRoot } from 'react-dom/client';
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import * as api from '../../api';
import type { Bookmark, SavedData, SavedHost } from '../../api/types';
import { useConnections } from '../../stores/connections';
import { usePanes } from '../../stores/panes';
import { useSettings } from '../../stores/settings';
import { localConnection } from '../../stores/testApi';
import { useUi } from '../../stores/ui';
import { ConnectionsDialog } from './ConnectionsDialog';

vi.mock('../../api', async () => (await import('../../stores/testApi')).createApiMock());

const onClose = vi.fn();
let root: Root;
let host: HTMLElement;

const box: SavedHost = {
  id: 'h1',
  name: 'Box',
  user: 'me',
  host: 'box.example',
  port: 2222,
  connId: 'ssh:me@box.example:2222',
};
const logs: Bookmark = { id: 'b1', connId: 'ssh:prod', path: '/srv/logs' };
const tmp: Bookmark = { id: 'b2', connId: 'ssh:prod', path: '/tmp', name: 'Scratch' };

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

function seed(saved: Partial<SavedData> = {}) {
  useConnections.setState({
    hosts: [{ alias: 'prod', source: 'c:1' }],
    savedHosts: saved.hosts ?? [],
    profiles: [{ id: 'p1', name: 'Archive', accessKeyId: 'AKIA', region: 'eu-west-1' }],
    prefs: saved.prefs ?? {},
    bookmarks: saved.bookmarks ?? [],
  });
}

function show(connId?: string) {
  act(() => root.render(<ConnectionsDialog {...(connId && { connId })} onClose={onClose} />));
}

beforeEach(() => {
  vi.clearAllMocks();
  usePanes.setState(usePanes.getInitialState(), true);
  useUi.setState(useUi.getInitialState(), true);
  useConnections.setState(useConnections.getInitialState(), true);
  useSettings.setState({
    settings: {
      globalTransfers: 3,
      perConnectionTransfers: 2,
      loadAppleKeychain: true,
      showHidden: false,
      swipeNavigation: true,
      theme: 'system',
    },
  });
  vi.mocked(api.connections.sshHostDetails).mockResolvedValue({
    hostName: 'prod.example',
    user: 'deploy',
    port: 22,
    proxyJump: 'bastion',
  });
  vi.mocked(api.connections.saveConnectionPrefs).mockResolvedValue(undefined);
  vi.mocked(api.connections.saved).mockResolvedValue({ hosts: [], prefs: {}, bookmarks: [] });
  vi.mocked(api.connections.connect).mockResolvedValue(localConnection());
  vi.mocked(api.files.list).mockReturnValue({ done: Promise.resolve(), cancel: vi.fn() });
  seed();
  host = document.createElement('div');
  document.body.append(host);
  root = createRoot(host);
});

afterEach(() => {
  act(() => root.unmount());
  host.remove();
});

const dialogs = () => [...host.querySelectorAll('dialog')];
const prompt = () => (dialogs().length > 1 ? dialogs()[1] : undefined);
const options = () => [...host.querySelectorAll<HTMLElement>('[role=option]')];
const option = (name: string) => options().find((o) => o.textContent?.startsWith(name))!;
const selected = () => options().find((o) => o.getAttribute('aria-selected') === 'true');

function button(label: string, within: ParentNode = dialogs()[0]!) {
  return [...within.querySelectorAll('button')].find(
    (b) => b.textContent === label || b.getAttribute('aria-label') === label
  )!;
}

const field = (label: string) =>
  [...host.querySelectorAll('label')]
    .find((l) => l.textContent?.startsWith(label))
    ?.querySelector<HTMLInputElement | HTMLSelectElement>('input, select');

function type(input: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!;
  act(() => {
    setter.call(input, value);
    input.dispatchEvent(new Event('input', { bubbles: true }));
  });
}

function choose(select: HTMLSelectElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value')!.set!;
  act(() => {
    setter.call(select, value);
    select.dispatchEvent(new Event('change', { bubbles: true }));
  });
}

const click = (el: HTMLElement) => act(() => el.click());
const radio = (label: string) =>
  host.querySelector<HTMLInputElement>(`input[type=radio][aria-label="${label}"]`)!;

const names = () =>
  [...host.querySelectorAll<HTMLInputElement>('input[aria-label^="Name for "]')].map((i) =>
    i.getAttribute('aria-label')!
  );
const pressKey = (el: HTMLElement, key: string) =>
  act(() => {
    el.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true }));
  });
const pointer = (target: EventTarget, kind: string, clientY: number) =>
  act(() => {
    target.dispatchEvent(
      new MouseEvent(kind, { bubbles: true, button: 0, clientY } as MouseEventInit)
    );
  });
// Rows are 40px tall, stacked from the top, so row n spans 40n to 40n + 40.
const layOut = () =>
  [...host.querySelectorAll<HTMLElement>('button[aria-label^="Move "]')].forEach((grip, n) => {
    grip.parentElement!.getBoundingClientRect = () => ({ top: n * 40, height: 40 }) as DOMRect;
  });

async function clickAsync(el: HTMLElement) {
  await act(async () => el.click());
}

function editProd() {
  click(radio('Red'));
  choose(field('Hidden files') as HTMLSelectElement, 'show');
}

describe('ConnectionsDialog', () => {
  it('starts on the connection it was opened for', () => {
    show('ssh:prod');
    expect(selected()?.textContent).toBe('prod');
  });

  it('starts on Local when opened without a connection', () => {
    show();
    expect(selected()?.textContent).toBe('Local');
  });

  it('groups connections as Local, SSH and S3', () => {
    show();
    const groups = [...host.querySelectorAll('[role=group]')].map((g) => g.textContent);
    expect(groups).toEqual(['LocalLocal', 'SSHprod', 'S3Archive']);
  });

  it('shows a config host read-only, with what ssh resolves for it', async () => {
    show('ssh:prod');
    await act(async () => {});
    const dialog = dialogs()[0]!;
    expect(dialog.textContent).toContain('SSH · from ~/.ssh/config');
    expect(dialog.textContent).toContain('deploy@prod.example:22');
    expect(dialog.textContent).toContain('ProxyJump bastion');
    expect(api.connections.sshHostDetails).toHaveBeenCalledWith('prod');
    for (const label of ['Name', 'Host', 'Port', 'User']) expect(field(label)).toBeUndefined();
    // Its app-side settings stay editable.
    expect(field('Open at')).toBeDefined();
    expect(field('Hidden files')).toBeDefined();
  });

  it('shows why ssh could not resolve a config host', async () => {
    vi.mocked(api.connections.sshHostDetails).mockRejectedValue(new Error('ssh not found'));
    show('ssh:prod');
    await act(async () => {});
    expect(dialogs()[0]!.textContent).toContain('ssh not found');
  });

  it('asks before leaving edits; Don’t save drops them', () => {
    show('ssh:prod');
    editProd();
    expect(option('prod').textContent).toContain('· edited');

    click(option('Local'));
    expect(prompt()?.textContent).toContain('Save changes to prod?');
    expect(selected()?.textContent).toContain('prod');

    click(button('Don’t save', prompt()));
    expect(prompt()).toBeUndefined();
    expect(selected()?.textContent).toBe('Local');
    expect(api.connections.saveConnectionPrefs).not.toHaveBeenCalled();

    click(option('prod'));
    expect(option('prod').textContent).not.toContain('edited');
    expect((field('Hidden files') as HTMLSelectElement).value).toBe('');
    expect(radio('Red').checked).toBe(false);
  });

  it('shows a bookmark saved elsewhere while the connection is untouched, but keeps edits', () => {
    show('ssh:prod');
    act(() => useConnections.setState({ bookmarks: [logs] }));
    expect(host.textContent).toContain('/srv/logs');
    expect(option('prod').textContent).not.toContain('edited');

    editProd();
    act(() => useConnections.setState({ bookmarks: [logs, tmp] }));
    expect(option('prod').textContent).toContain('· edited');
    expect(radio('Red').checked).toBe(true);
  });

  it('saves the edits when asked, then moves on', async () => {
    show('ssh:prod');
    editProd();
    click(option('Local'));

    await clickAsync(button('Save', prompt()));

    expect(api.connections.saveConnectionPrefs).toHaveBeenCalledWith(
      'ssh:prod',
      { color: 'red', showHidden: true },
      []
    );
    expect(selected()?.textContent).toBe('Local');
    expect(prompt()).toBeUndefined();
  });

  it('keeps the edits and the selection on Cancel', () => {
    show('ssh:prod');
    editProd();
    click(option('Local'));
    click(button('Cancel', prompt()));
    expect(prompt()).toBeUndefined();
    expect(selected()?.textContent).toContain('prod');
    expect(radio('Red').checked).toBe(true);
  });

  it('reverts edits with Revert and enables Save and Revert only while edited', () => {
    show('ssh:prod');
    expect(button('Save').disabled).toBe(true);
    expect(button('Revert').disabled).toBe(true);
    editProd();
    expect(button('Save').disabled).toBe(false);
    click(button('Revert'));
    expect(radio('Red').checked).toBe(false);
    expect(button('Save').disabled).toBe(true);
  });

  it('closes on Done, but asks first when there are unsaved edits', async () => {
    show('ssh:prod');
    click(button('Done'));
    expect(onClose).toHaveBeenCalledTimes(1);

    editProd();
    act(() => {
      dialogs()[0]!.dispatchEvent(new Event('cancel', { cancelable: true }));
    });
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(prompt()?.textContent).toContain('Save changes to prod?');

    await clickAsync(button('Don’t save', prompt()));
    expect(onClose).toHaveBeenCalledTimes(2);
  });

  it('saves a new SSH host first, then its settings under the id the backend gives it', async () => {
    const saved: SavedHost = { ...box, id: 'h9' };
    vi.mocked(api.connections.saveSSHHost).mockResolvedValue(saved);
    vi.mocked(api.connections.saved).mockResolvedValue({
      hosts: [saved],
      prefs: { [saved.connId]: { color: 'green' } as never },
      bookmarks: [],
    });
    show();
    click(button('Add connection'));
    click(button('SSH host'));
    expect(selected()?.textContent).toContain('the new SSH host');
    type(field('Name') as HTMLInputElement, 'Box');
    type(field('Host') as HTMLInputElement, 'box.example');
    type(field('Port') as HTMLInputElement, '2222');
    type(field('User') as HTMLInputElement, 'me');
    click(radio('Green'));

    await clickAsync(button('Save'));

    expect(api.connections.saveSSHHost).toHaveBeenCalledWith({
      id: '',
      name: 'Box',
      user: 'me',
      host: 'box.example',
      port: 2222,
      connId: '',
    });
    expect(api.connections.saveConnectionPrefs).toHaveBeenCalledWith(
      'ssh:me@box.example:2222',
      { color: 'green' },
      []
    );
    expect(vi.mocked(api.connections.saveSSHHost).mock.invocationCallOrder[0]).toBeLessThan(
      vi.mocked(api.connections.saveConnectionPrefs).mock.invocationCallOrder[0]!
    );
    expect(selected()?.textContent).toBe('Box');
    expect(button('Save').disabled).toBe(true);
  });

  it('does not save an incomplete host and says what is missing', async () => {
    show();
    click(button('Add connection'));
    click(button('SSH host'));
    type(field('Name') as HTMLInputElement, 'Box');
    await clickAsync(button('Save'));
    expect(api.connections.saveSSHHost).not.toHaveBeenCalled();
    expect(dialogs()[0]!.querySelector('[role=alert]')?.textContent).toMatch(/address/);
  });

  it('shows the backend’s error and keeps the draft when saving fails', async () => {
    vi.mocked(api.connections.saveConnectionPrefs).mockRejectedValue(new Error('disk full'));
    show('ssh:prod');
    editProd();
    await clickAsync(button('Save'));
    expect(dialogs()[0]!.querySelector('[role=alert]')?.textContent).toContain('disk full');
    expect(option('prod').textContent).toContain('· edited');
  });

  it('only allows removing saved hosts and S3 profiles', async () => {
    seed({ hosts: [box] });
    show();
    const remove = () => button('Remove connection') as HTMLButtonElement;
    expect(remove().disabled).toBe(true);
    click(option('prod'));
    expect(remove().disabled).toBe(true);
    click(option('Box'));
    expect(remove().disabled).toBe(false);

    click(remove());
    expect(dialogs()[0]!.textContent).toContain('Remove “Box”?');
    expect(api.connections.deleteSSHHost).not.toHaveBeenCalled();
    await clickAsync(button('Remove'));
    expect(api.connections.deleteSSHHost).toHaveBeenCalledWith('h1');
    expect(selected()?.textContent).toBe('Local');
  });

  it('keeps a connection when the removal is not confirmed', () => {
    seed({ hosts: [box] });
    show('ssh:me@box.example:2222');
    click(button('Remove connection'));
    click(button('Keep'));
    expect(api.connections.deleteSSHHost).not.toHaveBeenCalled();
    expect(button('Remove connection')).toBeDefined();
  });

  it('dims connections hidden from the picker and toggles them with the eye button', async () => {
    seed({ prefs: { 'ssh:prod': { hideInPicker: true } } });
    show('ssh:prod');
    expect(option('prod').querySelector('[title="Hidden from the picker"]')).not.toBeNull();
    expect(option('Archive').querySelector('[title="Hidden from the picker"]')).toBeNull();
    expect(button('Hide from picker').getAttribute('aria-pressed')).toBe('true');
    expect(button('Hide from picker').title).toBe('Hidden from the picker');

    click(button('Hide from picker'));
    expect(button('Hide from picker').getAttribute('aria-pressed')).toBe('false');
    expect(button('Hide from picker').title).toBe('Shown in the picker');
    expect(option('prod').querySelector('[title="Hidden from the picker"]')).toBeNull();
    await clickAsync(button('Save'));
    expect(api.connections.saveConnectionPrefs).toHaveBeenCalledWith('ssh:prod', {}, []);

    click(button('Hide from picker'));
    await clickAsync(button('Save'));
    expect(api.connections.saveConnectionPrefs).toHaveBeenLastCalledWith(
      'ssh:prod',
      { hideInPicker: true },
      []
    );
  });

  it('has no hide-from-picker button for Local', () => {
    show();
    expect(button('Hide from picker')).toBeUndefined();
  });

  it('favorites a connection, marks it in the list and saves it, Local included', async () => {
    show();
    expect(button('Favorite').getAttribute('aria-pressed')).toBe('false');
    expect(option('Local').querySelector('svg')).toBeNull();

    click(button('Favorite'));
    expect(button('Favorite').getAttribute('aria-pressed')).toBe('true');
    expect(option('Local').querySelector('svg')).not.toBeNull();
    await clickAsync(button('Save'));

    expect(api.connections.saveConnectionPrefs).toHaveBeenCalledWith(
      'local',
      { favorite: true },
      []
    );
  });

  it('edits parallel transfers and hidden files against the global defaults', async () => {
    show('ssh:prod');
    const transfers = field('Parallel transfers') as HTMLSelectElement;
    expect([...transfers.options].map((o) => o.textContent)).toEqual([
      'Use global setting (2)',
      '1',
      '2',
      '3',
    ]);
    choose(transfers, '3');
    choose(field('Hidden files') as HTMLSelectElement, 'show');
    await clickAsync(button('Save'));
    expect(api.connections.saveConnectionPrefs).toHaveBeenCalledWith(
      'ssh:prod',
      { transfers: 3, showHidden: true },
      []
    );
  });

  it('connects the active pane, after asking about unsaved edits', async () => {
    useUi.setState({ activePane: 'right' });
    show('ssh:prod');
    editProd();
    click(button('Connect'));
    expect(api.connections.connect).not.toHaveBeenCalled();
    expect(prompt()?.textContent).toContain('Save changes to prod?');

    await clickAsync(button('Don’t save', prompt()));

    expect(api.connections.connect).toHaveBeenCalledWith({ kind: 'ssh', alias: 'prod' });
    expect(usePanes.getState().panes.right.status.kind).not.toBe('idle');
    expect(onClose).toHaveBeenCalled();
  });

  it('cannot connect to an entry that is not saved yet', () => {
    show();
    click(button('Add connection'));
    click(button('SSH host'));
    expect((button('Connect') as HTMLButtonElement).disabled).toBe(true);
  });

  it('renames, pins and removes bookmarks, and adds one by path', async () => {
    seed({ bookmarks: [logs, tmp] });
    show('ssh:prod');
    click(button('Pin logs'));
    type(host.querySelector<HTMLInputElement>('input[aria-label="Name for /tmp"]')!, 'Temp');
    click(button('Remove bookmark Temp'));
    click(button('Remove'));
    type(
      host.querySelector<HTMLInputElement>('input[aria-label="Folder to bookmark"]')!,
      '/var/www/'
    );
    click(button('Add'));
    await clickAsync(button('Save'));

    expect(api.connections.saveConnectionPrefs).toHaveBeenCalledWith('ssh:prod', {}, [
      { id: 'b1', connId: 'ssh:prod', path: '/srv/logs', pinned: true },
      { id: '', connId: 'ssh:prod', path: '/var/www' },
    ]);
  });

  it('asks before removing a bookmark, and Keep cancels', () => {
    seed({ bookmarks: [logs, tmp] });
    show('ssh:prod');

    click(button('Remove bookmark Scratch'));
    expect(button('Remove')).toBeDefined();
    expect(host.textContent).toContain('/tmp');
    click(button('Keep'));

    expect(button('Remove')).toBeUndefined();
    expect(button('Remove bookmark Scratch')).toBeDefined();
    expect(host.querySelector('input[aria-label="Name for /tmp"]')).not.toBeNull();
    expect(option('prod').textContent).not.toContain('edited');

    click(button('Remove bookmark Scratch'));
    click(button('Remove'));
    expect(host.querySelector('input[aria-label="Name for /tmp"]')).toBeNull();
    expect(option('prod').textContent).toContain('edited');
  });

  describe('reordering bookmarks', () => {
    it('moves a row with the arrow keys on its handle, keeping focus there', async () => {
      seed({ bookmarks: [logs, tmp] });
      show('ssh:prod');
      expect(option('prod').textContent).not.toContain('edited');

      const grip = button('Move logs');
      grip.focus();
      pressKey(grip, 'ArrowDown');

      expect(names()).toEqual(['Name for /tmp', 'Name for /srv/logs']);
      expect(document.activeElement).toBe(button('Move logs'));
      expect(option('prod').textContent).toContain('edited');
      pressKey(button('Move logs'), 'ArrowDown');
      expect(names()).toEqual(['Name for /tmp', 'Name for /srv/logs']);
      pressKey(button('Move Scratch'), 'ArrowDown');
      pressKey(button('Move Scratch'), 'ArrowUp');
      expect(names()).toEqual(['Name for /tmp', 'Name for /srv/logs']);

      await clickAsync(button('Save'));
      expect(api.connections.saveConnectionPrefs).toHaveBeenCalledWith('ssh:prod', {}, [tmp, logs]);
    });

    it('drags a row by its handle, reflowing the others, and saves the new order', async () => {
      seed({ bookmarks: [logs, tmp, { id: 'b3', connId: 'ssh:prod', path: '/etc' }] });
      show('ssh:prod');
      layOut();

      pointer(button('Move logs'), 'pointerdown', 20);
      pointer(window, 'pointermove', 70);
      expect(names()).toEqual(['Name for /tmp', 'Name for /srv/logs', 'Name for /etc']);
      pointer(window, 'pointermove', 110);
      expect(names()).toEqual(['Name for /tmp', 'Name for /etc', 'Name for /srv/logs']);
      pointer(window, 'pointerup', 110);

      expect(names()).toEqual(['Name for /tmp', 'Name for /etc', 'Name for /srv/logs']);
      expect(option('prod').textContent).toContain('edited');
      await clickAsync(button('Save'));
      expect(api.connections.saveConnectionPrefs).toHaveBeenCalledWith('ssh:prod', {}, [
        tmp,
        { id: 'b3', connId: 'ssh:prod', path: '/etc' },
        logs,
      ]);
    });

    it('leaves the order alone when the drag is cancelled with Escape', () => {
      seed({ bookmarks: [logs, tmp] });
      show('ssh:prod');
      layOut();

      pointer(button('Move logs'), 'pointerdown', 20);
      pointer(window, 'pointermove', 70);
      act(() => {
        window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
      });
      pointer(window, 'pointerup', 70);

      expect(names()).toEqual(['Name for /srv/logs', 'Name for /tmp']);
      expect(option('prod').textContent).not.toContain('edited');
    });

    it('cancels a pending remove confirmation when a drag starts', () => {
      seed({ bookmarks: [logs, tmp] });
      show('ssh:prod');
      layOut();
      click(button('Remove bookmark logs'));
      expect(button('Keep')).toBeDefined();

      pointer(button('Move Scratch'), 'pointerdown', 60);
      pointer(window, 'pointerup', 60);

      expect(button('Keep')).toBeUndefined();
    });
  });

  it('puts the pin star after the name and before the remove button', () => {
    seed({ bookmarks: [logs] });
    show('ssh:prod');
    const row = button('Pin logs').parentElement!;
    const order = [...row.querySelectorAll('button, input')].map((el) =>
      el.getAttribute('aria-label')
    );
    expect(order).toEqual(['Move logs', 'Name for /srv/logs', 'Pin logs', 'Remove bookmark logs']);
  });

  it('offers Home folder and the draft’s bookmarks to open at, and saves the choice', async () => {
    seed({ bookmarks: [logs, tmp] });
    show('ssh:prod');
    const openAt = field('Open at') as HTMLSelectElement;
    expect([...openAt.options].map((o) => [o.value, o.textContent])).toEqual([
      ['', 'Home folder'],
      ['/srv/logs', 'logs — /srv/logs'],
      ['/tmp', 'Scratch — /tmp'],
    ]);

    choose(openAt, '/tmp');
    expect(host.textContent).toContain('Opens here');
    await clickAsync(button('Save'));

    expect(api.connections.saveConnectionPrefs).toHaveBeenCalledWith(
      'ssh:prod',
      { openAt: '/tmp' },
      [
        { id: 'b1', connId: 'ssh:prod', path: '/srv/logs' },
        { id: 'b2', connId: 'ssh:prod', path: '/tmp', name: 'Scratch' },
      ]
    );
  });

  it('opens at a bookmark added in the same draft', () => {
    show('ssh:prod');
    type(
      host.querySelector<HTMLInputElement>('input[aria-label="Folder to bookmark"]')!,
      '/var/www'
    );
    click(button('Add'));
    const openAt = field('Open at') as HTMLSelectElement;
    expect([...openAt.options].map((o) => o.value)).toEqual(['', '/var/www']);
  });

  it('resets Open at to Home folder when its bookmark is removed', async () => {
    seed({ bookmarks: [logs, tmp], prefs: { 'ssh:prod': { openAt: '/tmp' } } });
    show('ssh:prod');
    expect((field('Open at') as HTMLSelectElement).value).toBe('/tmp');
    expect(host.textContent).toContain('Opens here');

    click(button('Remove bookmark Scratch'));
    click(button('Remove'));

    expect((field('Open at') as HTMLSelectElement).value).toBe('');
    expect(host.textContent).not.toContain('Opens here');
    await clickAsync(button('Save'));
    expect(api.connections.saveConnectionPrefs).toHaveBeenCalledWith('ssh:prod', {}, [
      { id: 'b1', connId: 'ssh:prod', path: '/srv/logs' },
    ]);
  });

  it('refuses a relative bookmark path', () => {
    show('ssh:prod');
    type(host.querySelector<HTMLInputElement>('input[aria-label="Folder to bookmark"]')!, 'logs');
    expect((button('Add') as HTMLButtonElement).disabled).toBe(true);
  });

  it('saves an S3 profile, then its settings under s3:<id>', async () => {
    const profile = { id: 'p1', name: 'Archive 2', accessKeyId: 'AKIA', region: 'eu-west-1' };
    vi.mocked(api.connections.saveS3Profile).mockResolvedValue(profile);
    vi.mocked(api.connections.s3Profiles).mockResolvedValue([profile]);
    vi.mocked(api.connections.secretsSessionOnly).mockResolvedValue(false);
    show('s3:p1');
    type(field('Name') as HTMLInputElement, 'Archive 2');
    await clickAsync(button('Save'));

    expect(api.connections.saveS3Profile).toHaveBeenCalledWith(
      { id: 'p1', name: 'Archive 2', accessKeyId: 'AKIA', region: 'eu-west-1' },
      ''
    );
    expect(api.connections.saveConnectionPrefs).toHaveBeenCalledWith('s3:p1', {}, []);
    expect(selected()?.textContent).toBe('Archive 2');
  });

  it('leaves an S3 profile’s credentials alone when only a setting changed', async () => {
    show('s3:p1');
    click(radio('Blue'));
    await clickAsync(button('Save'));
    expect(api.connections.saveS3Profile).not.toHaveBeenCalled();
    expect(api.connections.saveConnectionPrefs).toHaveBeenCalledWith(
      's3:p1',
      { color: 'blue' },
      []
    );
  });

  it('filters the list by name', () => {
    show();
    type(host.querySelector<HTMLInputElement>('input[aria-label="Filter connections"]')!, 'arch');
    expect(options().map((o) => o.textContent)).toEqual(['Local', 'Archive']);
  });

  it('moves the selection with the arrow keys', () => {
    show();
    const list = host.querySelector<HTMLElement>('[role=listbox]')!;
    const press = (key: string) =>
      act(() => {
        list.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true }));
      });
    press('ArrowDown');
    expect(selected()?.textContent).toBe('prod');
    press('ArrowDown');
    expect(selected()?.textContent).toBe('Archive');
    press('ArrowDown');
    expect(selected()?.textContent).toBe('Archive');
    press('ArrowUp');
    expect(selected()?.textContent).toBe('prod');
    press('Home');
    expect(selected()?.textContent).toBe('Local');
  });

  it('reloads ~/.ssh/config: new aliases appear, a removed one falls back to Local', async () => {
    vi.mocked(api.connections.sshHosts).mockResolvedValue([{ alias: 'staging', source: 'c:1' }]);
    vi.mocked(api.connections.sshAvailable).mockResolvedValue('');
    show('ssh:prod');
    await act(async () => {});
    expect(api.connections.sshHostDetails).toHaveBeenCalledTimes(1);

    await clickAsync(button('Reload SSH config'));

    expect(options().map((o) => o.textContent)).toEqual(['Local', 'staging', 'Archive']);
    expect(selected()?.textContent).toBe('Local');
    expect(api.connections.saved).toHaveBeenCalled();
  });

  it('re-resolves the selected alias on reload and shows a read error inline', async () => {
    vi.mocked(api.connections.sshHosts).mockResolvedValueOnce([{ alias: 'prod', source: 'c:1' }]);
    vi.mocked(api.connections.sshAvailable).mockResolvedValue('');
    show('ssh:prod');
    await act(async () => {});

    await clickAsync(button('Reload SSH config'));
    expect(api.connections.sshHostDetails).toHaveBeenCalledTimes(2);
    expect(selected()?.textContent).toBe('prod');

    vi.mocked(api.connections.sshHosts).mockRejectedValueOnce(new Error('bad Include'));
    await clickAsync(button('Reload SSH config'));
    expect(dialogs()[0]!.textContent).toContain('bad Include');
    expect(options().map((o) => o.textContent)).toContain('prod');
  });
});
