import { describe, expect, it } from 'vitest';
import { Color } from '../api/enums';
import type { Bookmark } from '../api/types';
import { buildCatalog } from './catalog';
import {
  draftFor,
  hostFromFields,
  isDirty,
  newS3Draft,
  normalizeBookmarks,
  normalizePrefs,
  validateHostFields,
  moveBookmark,
  withBookmarks,
} from './connectionDraft';

const bookmark: Bookmark = { id: 'b1', connId: 'ssh:prod', path: '/srv/logs' };

const catalog = buildCatalog({
  hosts: [{ alias: 'prod', source: 'c:1' }],
  savedHosts: [
    {
      id: 'h1',
      name: 'Box',
      user: 'me',
      host: 'box.example',
      port: 2222,
      connId: 'ssh:me@box.example:2222',
    },
  ],
  profiles: [{ id: 'p1', name: 'Archive', accessKeyId: 'AKIA', region: 'eu-west-1' }],
  prefs: { 'ssh:prod': { color: Color.ColorRed } },
});
const saved = {
  savedHosts: [
    {
      id: 'h1',
      name: 'Box',
      user: 'me',
      host: 'box.example',
      port: 2222,
      connId: 'ssh:me@box.example:2222',
    },
  ],
  profiles: [{ id: 'p1', name: 'Archive', accessKeyId: 'AKIA', region: 'eu-west-1' }],
  bookmarks: [bookmark],
};
const draftOf = (id: string) =>
  draftFor(
    catalog.find((c) => c.id === id)!,
    saved
  );

describe('draftFor', () => {
  it('has nothing to edit about Local or an ssh config host', () => {
    expect(draftOf('local').identity.kind).toBe('fixed');
    const prod = draftOf('ssh:prod');
    expect(prod.identity.kind).toBe('fixed');
    expect(prod.prefs).toEqual({ color: 'red' });
    expect(prod.bookmarks).toEqual([bookmark]);
  });

  it('turns a saved host into editable text fields', () => {
    expect(draftOf('ssh:me@box.example:2222').identity).toEqual({
      kind: 'host',
      fields: { id: 'h1', name: 'Box', user: 'me', host: 'box.example', port: '2222' },
    });
  });

  it('turns an S3 profile into its form with the secret blank', () => {
    const { identity } = draftOf('s3:p1');
    expect(identity.kind).toBe('s3');
    if (identity.kind === 's3') {
      expect(identity.form).toMatchObject({
        id: 'p1',
        name: 'Archive',
        accessKeyId: 'AKIA',
        secret: '',
      });
    }
  });
});

describe('isDirty', () => {
  it('is clean for an untouched draft and for edits that normalise away', () => {
    const base = draftOf('ssh:prod');
    expect(isDirty(base, structuredClone(base))).toBe(false);
    expect(
      isDirty(base, { ...base, prefs: { ...base.prefs, favorite: false, transfers: 0 } })
    ).toBe(false);
    expect(isDirty(base, { ...base, prefs: { ...base.prefs, showHidden: null } })).toBe(false);
  });

  it('is dirty after changing a setting or a bookmark', () => {
    const base = draftOf('ssh:prod');
    const withBookmark = { ...base, bookmarks: [bookmark] };
    expect(isDirty(withBookmark, { ...withBookmark, prefs: { openAt: '/srv/logs' } })).toBe(true);
    expect(isDirty(base, { ...base, prefs: { ...base.prefs, favorite: true } })).toBe(true);
    expect(isDirty(base, { ...base, prefs: { ...base.prefs, showHidden: false } })).toBe(true);
    expect(isDirty(base, { ...base, bookmarks: [{ ...bookmark, name: 'Logs' }] })).toBe(true);
    expect(isDirty(base, { ...base, bookmarks: [{ ...bookmark, pinned: true }] })).toBe(true);
    expect(isDirty(base, { ...base, bookmarks: [] })).toBe(true);
    const two = { ...base, bookmarks: [bookmark, { ...bookmark, id: 'b2', path: '/tmp' }] };
    expect(isDirty(two, { ...two, bookmarks: moveBookmark(two.bookmarks, 0, 1) })).toBe(true);
  });

  it('is dirty after editing an address or typing an S3 secret', () => {
    const host = draftOf('ssh:me@box.example:2222');
    if (host.identity.kind !== 'host') throw new Error('expected a host');
    const renamed = {
      ...host,
      identity: { ...host.identity, fields: { ...host.identity.fields, name: 'Other' } },
    };
    expect(isDirty(host, renamed)).toBe(true);

    const s3 = draftOf('s3:p1');
    if (s3.identity.kind !== 's3') throw new Error('expected a profile');
    expect(
      isDirty(s3, {
        ...s3,
        identity: { ...s3.identity, form: { ...s3.identity.form, secret: 'x' } },
      })
    ).toBe(true);
  });
});

describe('moveBookmark', () => {
  const [a, b, c] = ['/a', '/b', '/c'].map((path, i) => ({ id: `b${i}`, connId: 'x', path }));

  it('moves an item down and up', () => {
    expect(moveBookmark([a!, b!, c!], 0, 2).map((x) => x.path)).toEqual(['/b', '/c', '/a']);
    expect(moveBookmark([a!, b!, c!], 2, 0).map((x) => x.path)).toEqual(['/c', '/a', '/b']);
  });

  it('does not mutate its input', () => {
    const list = [a!, b!];
    moveBookmark(list, 0, 1);
    expect(list).toEqual([a, b]);
  });
});

describe('normalising', () => {
  it('drops unset preferences and keeps the rest', () => {
    expect(
      normalizePrefs(
        { color: Color.ColorNone, openAt: '/srv/logs', transfers: 0, showHidden: null },
        [bookmark]
      )
    ).toEqual({
      openAt: '/srv/logs',
    });
    expect(
      normalizePrefs(
        {
          color: Color.ColorBlue,
          transfers: 3,
          showHidden: false,
          hideInPicker: true,
          favorite: true,
        },
        []
      )
    ).toEqual({
      color: 'blue',
      transfers: 3,
      showHidden: false,
      hideInPicker: true,
      favorite: true,
    });
  });

  it('drops an open-at folder that isn’t a bookmark', () => {
    expect(normalizePrefs({ openAt: '/srv/logs' }, [])).toEqual({});
  });

  it('resets open-at to the home folder when its bookmark is removed', () => {
    const base = { ...draftOf('ssh:prod'), bookmarks: [bookmark] };
    const set = { ...base, prefs: { ...base.prefs, openAt: '/srv/logs' } };
    expect(withBookmarks(set, [bookmark]).prefs.openAt).toBe('/srv/logs');
    const removed = withBookmarks(set, []);
    expect(removed.prefs.openAt).toBeUndefined();
    expect(removed.prefs.color).toBe(Color.ColorRed);
  });

  it('trims bookmark names and omits blank ones', () => {
    expect(
      normalizeBookmarks([
        { ...bookmark, name: '  ', pinned: false },
        { ...bookmark, path: '/b', name: ' B ', pinned: true },
      ])
    ).toEqual([bookmark, { id: 'b1', connId: 'ssh:prod', path: '/b', name: 'B', pinned: true }]);
  });
});

describe('saved host fields', () => {
  it('rejects a missing name, a missing address and a bad port', () => {
    const ok = { id: '', name: 'Box', user: '', host: 'box', port: '' };
    expect(validateHostFields(ok)).toBeNull();
    expect(validateHostFields({ ...ok, name: ' ' })).toMatch(/name/);
    expect(validateHostFields({ ...ok, host: '' })).toMatch(/address/);
    for (const port of ['0', '70000', 'ssh', '22.5']) {
      expect(validateHostFields({ ...ok, port })).toMatch(/port/);
    }
    expect(validateHostFields({ ...ok, port: '2222' })).toBeNull();
  });

  it('builds the host the backend stores, leaving empty parts out', () => {
    expect(hostFromFields({ id: '', name: ' Box ', user: ' ', host: ' box ', port: '' })).toEqual({
      id: '',
      name: 'Box',
      host: 'box',
      connId: '',
    });
    expect(
      hostFromFields({ id: 'h1', name: 'Box', user: 'me', host: 'box', port: '2222' })
    ).toEqual({
      id: 'h1',
      name: 'Box',
      user: 'me',
      host: 'box',
      port: 2222,
      connId: '',
    });
  });

  it('starts a new S3 draft as clean', () => {
    const blank = newS3Draft();
    expect(isDirty(blank, newS3Draft())).toBe(false);
  });
});
