import type { Bookmark, Prefs, S3Profile, SavedHost } from '../api/types';
import type { KnownConnection } from './catalog';
import { EMPTY_S3_FORM, type S3FormValues, formFromProfile } from './s3Form';

/** Selection keys of entries that aren't saved yet; saved ones are keyed by connection ID. */
export const NEW_HOST_KEY = 'new:host';
export const NEW_S3_KEY = 'new:s3';

export interface HostFields {
  /** Empty until the host is saved. */
  id: string;
  name: string;
  user: string;
  host: string;
  /** Text, so a half-typed port isn't lost; empty means the default. */
  port: string;
}

/** What the Connections dialog lets you edit about a connection itself, beyond its app settings. */
export type Identity =
  | { kind: 'fixed' }
  | { kind: 'host'; fields: HostFields }
  | { kind: 's3'; form: S3FormValues };

/** The unsaved state of the connection being edited. */
export interface Draft {
  identity: Identity;
  prefs: Prefs;
  bookmarks: Bookmark[];
}

export interface SavedState {
  savedHosts: readonly SavedHost[];
  profiles: readonly S3Profile[];
  bookmarks: readonly Bookmark[];
}

export const EMPTY_HOST_FIELDS: HostFields = { id: '', name: '', user: '', host: '', port: '' };

export function newHostDraft(): Draft {
  return { identity: { kind: 'host', fields: { ...EMPTY_HOST_FIELDS } }, prefs: {}, bookmarks: [] };
}

export function newS3Draft(): Draft {
  return { identity: { kind: 's3', form: { ...EMPTY_S3_FORM } }, prefs: {}, bookmarks: [] };
}

/** The saved state of a connection, as a draft to edit. */
export function draftFor(conn: KnownConnection, saved: SavedState): Draft {
  const bookmarks = saved.bookmarks.filter((b) => b.connId === conn.id).map((b) => ({ ...b }));
  const base = { prefs: { ...conn.prefs }, bookmarks };
  switch (conn.source) {
    case 'local':
    case 'config':
      return { ...base, identity: { kind: 'fixed' } };
    case 'saved': {
      const host = saved.savedHosts.find((h) => h.connId === conn.id);
      const fields: HostFields = {
        id: host?.id ?? '',
        name: conn.name,
        user: host?.user ?? '',
        host: host?.host ?? '',
        port: host?.port ? String(host.port) : '',
      };
      return { ...base, identity: { kind: 'host', fields } };
    }
    case 's3': {
      const profile = saved.profiles.find((p) => `s3:${p.id}` === conn.id);
      const form = profile ? formFromProfile(profile) : { ...EMPTY_S3_FORM, name: conn.name };
      return { ...base, identity: { kind: 's3', form } };
    }
    default: {
      const unreachable: never = conn.source;
      return unreachable;
    }
  }
}

/** Prefs as the backend stores them: unset fields are absent, not empty. */
export function normalizePrefs(p: Prefs): Prefs {
  const openAt = (p.openAt ?? '').trim();
  return {
    ...(p.color && { color: p.color }),
    ...(openAt && { openAt }),
    ...(p.transfers && { transfers: p.transfers }),
    ...(p.showHidden != null && { showHidden: p.showHidden }),
    ...(p.hideInPicker && { hideInPicker: true }),
  };
}

export function normalizeBookmarks(list: readonly Bookmark[]): Bookmark[] {
  return list.map((b) => {
    const name = (b.name ?? '').trim();
    return {
      id: b.id,
      connId: b.connId,
      path: b.path,
      ...(name && { name }),
      ...(b.pinned && { pinned: true }),
    };
  });
}

export function identityChanged(base: Draft, draft: Draft): boolean {
  return JSON.stringify(base.identity) !== JSON.stringify(draft.identity);
}

export function prefsChanged(base: Draft, draft: Draft): boolean {
  return (
    JSON.stringify([normalizePrefs(base.prefs), normalizeBookmarks(base.bookmarks)]) !==
    JSON.stringify([normalizePrefs(draft.prefs), normalizeBookmarks(draft.bookmarks)])
  );
}

export function isDirty(base: Draft, draft: Draft): boolean {
  return identityChanged(base, draft) || prefsChanged(base, draft);
}

const MAX_PORT = 65535;

/** Returns an error message, or null when the host can be saved. */
export function validateHostFields(f: HostFields): string | null {
  if (f.name.trim() === '') return 'Give the host a name.';
  if (f.host.trim() === '') return 'Enter the host address.';
  const port = f.port.trim();
  if (port !== '' && !(/^\d+$/.test(port) && Number(port) >= 1 && Number(port) <= MAX_PORT)) {
    return `The port must be a number from 1 to ${MAX_PORT}.`;
  }
  return null;
}

/** The backend derives `connId` from the address, so it's left empty here. */
export function hostFromFields(f: HostFields): SavedHost {
  const user = f.user.trim();
  const port = Number(f.port.trim());
  return {
    id: f.id,
    name: f.name.trim(),
    ...(user && { user }),
    host: f.host.trim(),
    ...(port && { port }),
    connId: '',
  };
}
