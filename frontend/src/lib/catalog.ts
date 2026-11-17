import { targets } from '../api/targets';
import type { Bookmark, Host, Prefs, S3Profile, SavedHost, Target } from '../api/types';
import { baseName } from './paths';
import { sortByLabel } from './sort';

/** Where a connection is defined, which decides what the app may edit about it. */
export type ConnSource = 'local' | 'config' | 'saved' | 's3';

/** A connection the app knows by name: Local, a ~/.ssh/config alias, a saved host or an S3 profile. */
export interface KnownConnection {
  /** Connection ID: the key for preferences and bookmarks. */
  id: string;
  source: ConnSource;
  /** Alias, saved host name, profile name or "Local". */
  name: string;
  target: Target;
  prefs: Prefs;
}

export const LOCAL_ID = 'local';

export const EMPTY_PREFS: Prefs = Object.freeze({});

export interface CatalogInput {
  hosts: readonly Host[];
  savedHosts: readonly SavedHost[];
  profiles: readonly S3Profile[];
  prefs: Partial<Record<string, Prefs>>;
}

/** Every known connection: Local, then SSH hosts by name, then S3 profiles by name. */
export function buildCatalog({
  hosts,
  savedHosts,
  profiles,
  prefs,
}: CatalogInput): KnownConnection[] {
  const known = (
    id: string,
    source: ConnSource,
    name: string,
    target: Target
  ): KnownConnection => ({
    id,
    source,
    name,
    target,
    prefs: prefs[id] ?? EMPTY_PREFS,
  });

  const ssh = [
    ...hosts.map((h) => known(`ssh:${h.alias}`, 'config', h.alias, targets.ssh(h.alias))),
    ...savedHosts.map((h) =>
      known(h.connId, 'saved', h.name, targets.quick(h.user ?? '', h.host, h.port ?? 0))
    ),
  ];
  const s3 = profiles.map((p) => known(`s3:${p.id}`, 's3', p.name, targets.s3(p.id)));

  return [
    known(LOCAL_ID, 'local', 'Local', targets.local()),
    ...sortByLabel(ssh, (c) => c.name),
    ...sortByLabel(s3, (c) => c.name),
  ];
}

/** Connections offered in the picker: Local always, the rest unless hidden. */
export function pickerConnections(catalog: readonly KnownConnection[]): KnownConnection[] {
  return catalog.filter((c) => c.source === 'local' || !c.prefs.hideInPicker);
}

export interface PickerSection {
  id: 'favorites' | 'local' | 'ssh' | 's3';
  label: string;
  items: KnownConnection[];
}

/** The picker's groups: favorites first, then Local, SSH hosts and S3, each without favorites. */
export function pickerSections(catalog: readonly KnownConnection[]): PickerSection[] {
  const offered = pickerConnections(catalog);
  const rest = offered.filter((c) => !c.prefs.favorite);
  const sections: PickerSection[] = [
    { id: 'favorites', label: 'Favorites', items: offered.filter((c) => c.prefs.favorite) },
    { id: 'local', label: 'Local', items: rest.filter((c) => c.source === 'local') },
    {
      id: 'ssh',
      label: 'SSH hosts',
      items: rest.filter((c) => c.source === 'config' || c.source === 'saved'),
    },
    { id: 's3', label: 'S3', items: rest.filter((c) => c.source === 's3') },
  ];
  return sections.filter((s) => s.items.length > 0);
}

export function bookmarkLabel(b: Bookmark): string {
  return b.name || baseName(b.path) || b.path;
}

export function bookmarkAt(
  bookmarks: readonly Bookmark[],
  connId: string,
  path: string
): Bookmark | undefined {
  return bookmarks.find((b) => b.connId === connId && b.path === path);
}
