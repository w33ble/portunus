import { type KnownConnection, pickerSections } from '../../lib/catalog';
import { matchName } from '../../lib/match';
import type { IconName } from '../ui/Icon';

export interface PickerRow {
  conn: KnownConnection;
  /** Left out of the picker unless revealed: drawn dimmed. */
  hidden: boolean;
}

export interface PickerGroup {
  id: string;
  /** Null for the pane's own connection when no section lists it. */
  label: string | null;
  rows: PickerRow[];
  /** Non-interactive line under the heading, e.g. why SSH is unavailable. */
  notice?: string;
}

export interface GroupOptions {
  /** The pane's connection, listed first when no section offers it. */
  current: KnownConnection | null;
  query: string;
  showHidden: boolean;
  sshUnavailable: string;
}

const isHidden = (c: KnownConnection) => c.source !== 'local' && c.prefs.hideInPicker === true;

export function hiddenCount(catalog: readonly KnownConnection[]): number {
  return catalog.filter(isHidden).length;
}

export function sourceIcon(source: KnownConnection['source']): IconName {
  switch (source) {
    case 'local':
      return 'home';
    case 'config':
    case 'saved':
      return 'server';
    case 's3':
      return 'cloud';
    default: {
      const unreachable: never = source;
      return unreachable;
    }
  }
}

/** The picker's sections for a query, with hidden connections only when asked for. */
export function buildGroups(
  catalog: readonly KnownConnection[],
  { current, query, showHidden, sshUnavailable }: GroupOptions
): PickerGroup[] {
  const hiddenIds = new Set(catalog.filter(isHidden).map((c) => c.id));
  const source = showHidden
    ? catalog.map((c) =>
        hiddenIds.has(c.id) ? { ...c, prefs: { ...c.prefs, hideInPicker: false } } : c
      )
    : catalog;
  const sections = pickerSections(source);

  const needle = query.trim();
  const matches = (c: KnownConnection) => needle === '' || matchName(c.name, needle) !== null;
  const row = (conn: KnownConnection): PickerRow => ({ conn, hidden: hiddenIds.has(conn.id) });

  const notice = needle === '' && sshUnavailable !== '' ? sshUnavailable : undefined;
  const groups: PickerGroup[] = sections.map((s) => ({
    id: s.id,
    label: s.label,
    rows: s.items.filter(matches).map(row),
    ...(s.id === 'ssh' && notice !== undefined && { notice }),
  }));

  if (notice !== undefined && !groups.some((g) => g.id === 'ssh')) {
    const at = groups.findIndex((g) => g.id === 's3');
    groups.splice(at < 0 ? groups.length : at, 0, {
      id: 'ssh',
      label: 'SSH hosts',
      rows: [],
      notice,
    });
  }

  const listed = sections.some((s) => s.items.some((c) => c.id === current?.id));
  if (current && !listed && matches(current)) {
    groups.unshift({ id: 'current', label: null, rows: [row(current)] });
  }
  return groups.filter((g) => g.rows.length > 0 || g.notice !== undefined);
}
