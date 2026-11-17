import { act } from 'react';
import { type Root, createRoot } from 'react-dom/client';
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { Color, Kind, TargetKind } from '../../api/enums';
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

const trigger = () => host.querySelector<HTMLButtonElement>('button[aria-label="Connection"]')!;
const search = () => host.querySelector<HTMLInputElement>('input[role="combobox"]')!;
const activeName = () =>
  document.getElementById(search().getAttribute('aria-activedescendant')!)?.textContent;
const options = () => [...host.querySelectorAll('[role=option]')];
const names = () => options().map((o) => o.textContent);
const action = (label: string) =>
  [...host.querySelectorAll<HTMLButtonElement>('button')].find((b) => b.textContent === label)!;

/** Option names by section heading, in display order. */
function sections() {
  return [...host.querySelectorAll('[role=group]')].map((g) => ({
    heading: g.firstElementChild?.textContent,
    names: [...g.querySelectorAll('[role=option]')].map((o) => o.textContent),
  }));
}

function renderPicker() {
  act(() => root.render(<ConnectionPicker paneId="left" />));
}

function openPicker() {
  renderPicker();
  act(() => trigger().click());
}

function press(key: string, target: HTMLElement = search()) {
  act(() => {
    target.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true }));
  });
}

function type(value: string) {
  act(() => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set?.call(
      search(),
      value
    );
    search().dispatchEvent(new Event('input', { bubbles: true }));
  });
}

function pointOnPane(connId: string, lastLabel?: string) {
  usePanes.setState((s) => ({
    panes: {
      ...s.panes,
      left: {
        ...s.panes.left,
        connId,
        ...(lastLabel !== undefined && {
          lastTarget: {
            target: { kind: TargetKind.TargetS3, profileId: 'gone' },
            label: lastLabel,
          },
        }),
      },
    },
  }));
}

const savedHost = (id: string, name: string, connId: string) => ({
  id,
  name,
  host: `${id}.example`,
  connId,
});

describe('ConnectionPicker trigger', () => {
  it('shows Local with no color before the pane connects', () => {
    renderPicker();

    expect(trigger().textContent).toBe('Local');
    expect(trigger().querySelector('[title$=" tag"]')).toBeNull();
    expect(trigger().getAttribute('aria-haspopup')).toBe('listbox');
    expect(trigger().getAttribute('aria-expanded')).toBe('false');
  });

  it('shows the pane’s connection name and color', () => {
    useConnections.setState({
      profiles: [{ id: 'p1', name: 'Prod bucket' }] as never,
      prefs: { 's3:p1': { color: Color.ColorRed } },
    });
    pointOnPane('s3:p1');
    renderPicker();

    expect(trigger().textContent).toBe('Prod bucket');
    expect(trigger().querySelector<HTMLElement>('[title="red tag"]')?.style.backgroundColor).toBe(
      'var(--tag-red)'
    );
  });

  it('names a dropped connection from the pane’s last target', () => {
    pointOnPane('s3:gone', 'Archive');
    renderPicker();

    expect(trigger().textContent).toBe('Archive');
  });

  it('names a live quick connection from the connection list', () => {
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
    pointOnPane('ssh:me@h:22');
    renderPicker();

    expect(trigger().textContent).toBe('me@h');
  });

  it('opens with ArrowDown and focuses the search field', () => {
    renderPicker();

    press('ArrowDown', trigger());

    expect(trigger().getAttribute('aria-expanded')).toBe('true');
    expect(document.activeElement).toBe(search());
    expect(search().placeholder).toBe('Connect to…');
  });
});

describe('ConnectionPicker list', () => {
  it('lists SSH hosts and S3 profiles in case-insensitive natural order', () => {
    useConnections.setState({
      hosts: ['web10', 'Zeta', 'alpha', 'web2', 'Beta'].map((alias) => ({ alias, source: 'c:1' })),
      profiles: [
        { id: 'p1', name: 'logs' },
        { id: 'p2', name: 'Archive' },
      ] as never,
    });
    openPicker();

    expect(sections()).toEqual([
      { heading: 'Local', names: ['Local'] },
      { heading: 'SSH hosts', names: ['alpha', 'Beta', 'web2', 'web10', 'Zeta'] },
      { heading: 'S3', names: ['Archive', 'logs'] },
    ]);
  });

  it('lists favorites first and does not repeat them in their own group', () => {
    useConnections.setState({
      hosts: [{ alias: 'web', source: 'c:1' }],
      savedHosts: [savedHost('h1', 'Alpha box', 'ssh:a.example')],
      profiles: [{ id: 'p1', name: 'Prod bucket' }] as never,
      prefs: { 's3:p1': { favorite: true }, 'ssh:web': { favorite: true } },
    });
    openPicker();

    expect(sections()).toEqual([
      { heading: 'Favorites', names: ['web', 'Prod bucket'] },
      { heading: 'Local', names: ['Local'] },
      { heading: 'SSH hosts', names: ['Alpha box'] },
    ]);
  });

  it('keeps hidden connections out until Show reveals them dimmed', () => {
    useConnections.setState({
      savedHosts: [
        savedHost('h1', 'Alpha box', 'ssh:a.example'),
        savedHost('h2', 'Secret box', 'ssh:s.example'),
      ],
      profiles: [{ id: 'p1', name: 'Quiet bucket' }] as never,
      prefs: { 'ssh:s.example': { hideInPicker: true }, 's3:p1': { hideInPicker: true } },
    });
    openPicker();

    expect(names()).toEqual(['Local', 'Alpha box']);
    expect(host.querySelector('[data-hidden="true"]')).toBeNull();

    act(() => action('2 hidden · Show').click());

    expect(sections()).toEqual([
      { heading: 'Local', names: ['Local'] },
      { heading: 'SSH hosts', names: ['Alpha box', 'Secret box'] },
      { heading: 'S3', names: ['Quiet bucket'] },
    ]);
    expect([...host.querySelectorAll('[data-hidden="true"]')].map((o) => o.textContent)).toEqual([
      'Secret box',
      'Quiet bucket',
    ]);
    expect(action('2 hidden · Hide')).toBeDefined();
    expect(document.activeElement).toBe(search());
  });

  it('shows no footer when nothing is hidden', () => {
    openPicker();

    expect(
      [...host.querySelectorAll('button')].some((b) => /hidden/.test(b.textContent ?? ''))
    ).toBe(false);
  });

  it('lists a hidden connection first while the pane is on it', () => {
    useConnections.setState({
      savedHosts: [savedHost('h2', 'Secret box', 'ssh:s.example')],
      prefs: { 'ssh:s.example': { hideInPicker: true } },
    });
    pointOnPane('ssh:s.example');
    openPicker();

    expect(names()).toEqual(['Secret box', 'Local']);
    expect(options()[0]?.getAttribute('aria-selected')).toBe('true');
  });

  it('lists an unsaved quick connection first and marks it current', () => {
    pointOnPane('s3:gone', 'Archive');
    openPicker();

    expect(names()).toEqual(['Archive', 'Local']);
    expect(options().map((o) => o.getAttribute('aria-selected'))).toEqual(['true', 'false']);
  });

  it('marks only the pane’s connection as selected', () => {
    useConnections.setState({ profiles: [{ id: 'p1', name: 'Prod' }] as never });
    pointOnPane('s3:p1');
    openPicker();

    const selected = options().filter((o) => o.getAttribute('aria-selected') === 'true');
    expect(selected.map((o) => o.textContent)).toEqual(['Prod']);
  });

  it('shows why SSH is unavailable as a row that cannot be chosen', () => {
    useConnections.setState({ sshUnavailable: 'ssh not found' });
    openPicker();

    expect(sections().map((s) => s.heading)).toEqual(['Local', 'SSH hosts']);
    expect(host.textContent).toContain('ssh not found');
    expect(names()).toEqual(['Local']);
  });
});

describe('ConnectionPicker filter', () => {
  beforeEach(() => {
    useConnections.setState({
      hosts: [{ alias: 'web', source: 'c:1' }],
      savedHosts: [savedHost('h1', 'Café box', 'ssh:c.example')],
      profiles: [{ id: 'p1', name: 'Prod bucket' }] as never,
    });
  });

  it('narrows the list and drops sections with no match', () => {
    openPicker();

    type('prod');

    expect(sections()).toEqual([{ heading: 'S3', names: ['Prod bucket'] }]);
  });

  it('ignores case and accents', () => {
    openPicker();

    type('CAFE');

    expect(names()).toEqual(['Café box']);
  });

  it('says so when nothing matches', () => {
    openPicker();

    type('zzzz');

    expect(options()).toEqual([]);
    expect(host.textContent).toContain('No matching connections');
  });

  it('connects to the first match with Enter', () => {
    const connect = vi.spyOn(usePanes.getState(), 'connectTo').mockResolvedValue(true);
    openPicker();

    type('prod');
    press('Enter');

    expect(connect).toHaveBeenCalledWith('left', { kind: 's3', profileId: 'p1' }, 'Prod bucket');
  });
});

describe('ConnectionPicker keyboard and pointer', () => {
  beforeEach(() => {
    useConnections.setState({
      hosts: ['alpha', 'beta'].map((alias) => ({ alias, source: 'c:1' })),
    });
  });

  it('connects to the next option with ArrowDown and Enter, then closes', () => {
    const connect = vi.spyOn(usePanes.getState(), 'connectTo').mockResolvedValue(true);
    openPicker();

    press('ArrowDown');
    press('Enter');

    expect(connect).toHaveBeenCalledTimes(1);
    expect(connect).toHaveBeenCalledWith('left', { kind: 'ssh', alias: 'alpha' }, 'alpha');
    expect(options()).toEqual([]);
    expect(document.activeElement).toBe(trigger());
  });

  it('points aria-activedescendant at the active option and wraps', () => {
    openPicker();
    expect(activeName()).toBe('Local');
    press('ArrowDown');
    expect(activeName()).toBe('alpha');
    press('End');
    expect(activeName()).toBe('Manage connections…');
    press('ArrowDown');
    expect(activeName()).toBe('Local');
    press('ArrowUp');
    expect(activeName()).toBe('Manage connections…');
    press('Home');
    expect(activeName()).toBe('Local');
  });

  it('starts on the pane’s own connection', () => {
    pointOnPane('ssh:beta');
    openPicker();

    const active = document.getElementById(search().getAttribute('aria-activedescendant')!);
    expect(active?.textContent).toBe('beta');
  });

  it('does not reconnect to the connection the pane is already on', () => {
    const connect = vi.spyOn(usePanes.getState(), 'connectTo').mockResolvedValue(true);
    pointOnPane('ssh:beta');
    openPicker();

    press('Enter');

    expect(connect).not.toHaveBeenCalled();
    expect(options()).toEqual([]);
  });

  it('runs an action reached with the arrow keys', () => {
    openPicker();

    press('ArrowUp');
    press('Enter');

    expect(useUi.getState().dialog).toEqual({ kind: 'connections' });
  });

  it('connects to a clicked row', () => {
    const connect = vi.spyOn(usePanes.getState(), 'connectTo').mockResolvedValue(true);
    openPicker();

    act(() => (options().find((o) => o.textContent === 'beta') as HTMLElement).click());

    expect(connect).toHaveBeenCalledWith('left', { kind: 'ssh', alias: 'beta' }, 'beta');
    expect(options()).toEqual([]);
  });

  it('closes on Escape and returns focus to the trigger', () => {
    openPicker();

    press('Escape');

    expect(options()).toEqual([]);
    expect(trigger().getAttribute('aria-expanded')).toBe('false');
    expect(document.activeElement).toBe(trigger());
  });

  it('closes on Tab', () => {
    openPicker();

    press('Tab');

    expect(options()).toEqual([]);
  });

  it('closes on a press outside and toggles from the trigger', () => {
    openPicker();

    act(() => {
      document.body.dispatchEvent(new Event('pointerdown', { bubbles: true }));
    });
    expect(options()).toEqual([]);

    act(() => trigger().click());
    expect(options().length).toBeGreaterThan(0);
    act(() => trigger().click());
    expect(options()).toEqual([]);
  });
});

describe('ConnectionPicker actions', () => {
  it('opens the connections dialog on the pane’s connection', () => {
    useConnections.setState({ profiles: [{ id: 'p1', name: 'Prod' }] as never });
    pointOnPane('s3:p1');
    openPicker();

    act(() => action('Manage connections…').click());

    expect(useUi.getState().dialog).toEqual({ kind: 'connections', connId: 's3:p1' });
    expect(options()).toEqual([]);
  });

  it('opens quick connect for the pane', () => {
    openPicker();

    act(() => action('Quick connect…').click());

    expect(useUi.getState().dialog).toEqual({ kind: 'quickConnect', paneId: 'left' });
  });
});
