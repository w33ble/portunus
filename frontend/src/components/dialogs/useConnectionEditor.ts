import { useState } from 'react';
import { toAppError } from '../../api/errors';
import { type KnownConnection, LOCAL_ID } from '../../lib/catalog';
import {
  type Draft,
  type Identity,
  NEW_HOST_KEY,
  NEW_S3_KEY,
  type SavedState,
  draftFor,
  hostFromFields,
  identityChanged,
  isDirty,
  newHostDraft,
  newS3Draft,
  normalizeBookmarks,
  normalizePrefs,
  validateHostFields,
} from '../../lib/connectionDraft';
import { profileFromForm, secretFor, validateS3Form } from '../../lib/s3Form';
import { catalogNow, useCatalog, useConnections } from '../../stores/connections';
import { usePanes } from '../../stores/panes';
import { useUi } from '../../stores/ui';

export type NewKind = 'host' | 's3';

/** What to do once the draft is saved or discarded; it gets the (possibly new) connection ID. */
type Continuation = (key: string) => void | Promise<void>;

function savedNow(): SavedState {
  const { savedHosts, profiles, bookmarks } = useConnections.getState();
  return { savedHosts, profiles, bookmarks };
}

/** The saved state of the entry with this key; an unknown key falls back to Local. */
function open(
  key: string,
  catalog: readonly KnownConnection[],
  saved: SavedState
): { key: string; draft: Draft } {
  if (key === NEW_HOST_KEY) return { key, draft: newHostDraft() };
  if (key === NEW_S3_KEY) return { key, draft: newS3Draft() };
  const conn = catalog.find((c) => c.id === key) ?? catalog.find((c) => c.id === LOCAL_ID);
  if (conn === undefined) {
    return { key: LOCAL_ID, draft: { identity: { kind: 'fixed' }, prefs: {}, bookmarks: [] } };
  }
  return { key: conn.id, draft: draftFor(conn, saved) };
}

/** `open` against the stores as they are now, for handlers. */
function openNow(key: string): { key: string; draft: Draft } {
  return open(key, catalogNow(), savedNow());
}

export interface ConnectionEditor {
  catalog: readonly KnownConnection[];
  key: string;
  /** The connection being edited; null for an entry that isn't saved yet. */
  conn: KnownConnection | null;
  draft: Draft;
  dirty: boolean;
  /** What the draft is called: its name, else a placeholder. */
  label: string;
  busy: boolean;
  error: string | null;
  /** Set while the "Save changes?" prompt is up. */
  asking: boolean;
  select: (key: string) => void;
  add: (kind: NewKind) => void;
  setIdentity: (identity: Identity) => void;
  setPrefs: (patch: Partial<Draft['prefs']>) => void;
  setBookmarks: (bookmarks: Draft['bookmarks']) => void;
  revert: () => void;
  save: () => Promise<string | null>;
  remove: () => Promise<void>;
  connect: () => void;
  done: () => void;
  answer: (choice: 'save' | 'discard' | 'cancel') => Promise<void>;
}

export function useConnectionEditor(
  initialId: string | undefined,
  onClose: () => void
): ConnectionEditor {
  const catalog = useCatalog();
  const savedHosts = useConnections((s) => s.savedHosts);
  const profiles = useConnections((s) => s.profiles);
  const bookmarks = useConnections((s) => s.bookmarks);
  const [{ key, draft: edited, from }, setState] = useState<{
    key: string;
    draft: Draft;
    /** What the draft was opened from; null once it holds edits that must not be replaced. */
    from: Draft | null;
  }>(() => {
    const opened = openNow(initialId ?? LOCAL_ID);
    return { ...opened, from: opened.draft };
  });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [after, setAfter] = useState<{ run: Continuation } | null>(null);

  const conn = catalog.find((c) => c.id === key) ?? null;
  const base = open(key, catalog, { savedHosts, profiles, bookmarks }).draft;
  // An untouched draft follows saved data that changes underneath, like a bookmark added with ⌘D.
  const draft = from !== null && !isDirty(from, edited) ? base : edited;
  const dirty = isDirty(base, draft);

  const identityName =
    draft.identity.kind === 'host'
      ? draft.identity.fields.name
      : draft.identity.kind === 's3'
        ? draft.identity.form.name
        : '';
  const label =
    identityName.trim() ||
    conn?.name ||
    (draft.identity.kind === 's3' ? 'the new S3 profile' : 'the new SSH host');

  const edit = (next: { key: string; draft: Draft }) => {
    setState({ ...next, from: next.draft });
    setError(null);
  };

  const change = (next: Draft) => {
    setState({ key, draft: next, from: draft });
    setError(null);
  };

  const guard = (run: Continuation) => {
    if (dirty) setAfter({ run });
    else void run(key);
  };

  const run = async (work: () => Promise<void>): Promise<boolean> => {
    setBusy(true);
    setError(null);
    try {
      await work();
      return true;
    } catch (e) {
      setError(toAppError(e).message);
      return false;
    } finally {
      setBusy(false);
    }
  };

  const save = async (): Promise<string | null> => {
    const { identity } = draft;
    const invalid =
      identity.kind === 'host'
        ? validateHostFields(identity.fields)
        : identity.kind === 's3'
          ? validateS3Form(identity.form)
          : null;
    if (invalid) {
      setError(invalid);
      return null;
    }
    let saved = key;
    const ok = await run(async () => {
      const store = useConnections.getState();
      const changed = identityChanged(base, draft);
      let next: Identity = identity;
      switch (identity.kind) {
        case 'fixed':
          break;
        case 'host': {
          if (identity.fields.id === '' || changed) {
            const host = await store.saveHost(hostFromFields(identity.fields));
            saved = host.connId;
            next = { kind: 'host', fields: { ...identity.fields, id: host.id } };
          }
          break;
        }
        case 's3': {
          if (identity.form.id === '' || changed) {
            const profile = await store.saveProfile(
              profileFromForm(identity.form),
              secretFor(identity.form)
            );
            saved = `s3:${profile.id}`;
            next = { kind: 's3', form: { ...identity.form, id: profile.id, secret: '' } };
          }
          break;
        }
        default: {
          const unreachable: never = identity;
          return unreachable;
        }
      }
      // The entry exists now: keep following it if saving the preferences below fails.
      if (next !== identity)
        setState({ key: saved, draft: { ...draft, identity: next }, from: null });
      await store.savePrefs(
        saved,
        normalizePrefs(draft.prefs),
        normalizeBookmarks(draft.bookmarks)
      );
      edit(openNow(saved));
    });
    return ok ? saved : null;
  };

  const answer = async (choice: 'save' | 'discard' | 'cancel') => {
    const pending = after;
    setAfter(null);
    if (pending === null || choice === 'cancel') return;
    if (choice === 'discard') {
      await pending.run(key);
      return;
    }
    const saved = await save();
    if (saved !== null) await pending.run(saved);
  };

  return {
    catalog,
    key,
    conn,
    draft,
    dirty,
    label,
    busy,
    error,
    asking: after !== null,
    select: (next) => {
      if (next !== key) guard(() => edit(openNow(next)));
    },
    add: (kind) => guard(() => edit(openNow(kind === 'host' ? NEW_HOST_KEY : NEW_S3_KEY))),
    setIdentity: (identity) => change({ ...draft, identity }),
    setPrefs: (patch) => change({ ...draft, prefs: { ...draft.prefs, ...patch } }),
    setBookmarks: (next) => change({ ...draft, bookmarks: next }),
    revert: () => edit({ key, draft: base }),
    save,
    async remove() {
      const { identity } = draft;
      if (identity.kind === 'fixed') return;
      const store = useConnections.getState();
      const ok = await run(async () => {
        if (conn !== null && identity.kind === 'host') await store.deleteHost(identity.fields.id);
        else if (conn !== null && identity.kind === 's3')
          await store.deleteProfile(identity.form.id);
      });
      if (ok) edit(openNow(LOCAL_ID));
    },
    connect: () =>
      guard((k) => {
        const target = catalogNow().find((c) => c.id === k);
        if (target === undefined) return;
        void usePanes.getState().connectTo(useUi.getState().activePane, target.target, target.name);
        onClose();
      }),
    done: () => guard(onClose),
    answer,
  };
}
