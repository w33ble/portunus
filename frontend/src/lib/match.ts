import { type Result, go, single } from 'fuzzysort';
import type { Entry } from '../api/types';

/** Case-folded code points after NFC, so NFD names (common on macOS) match typed text. */
export function foldChars(s: string): string[] {
  return [...s.normalize('NFC')].map((c) => c.toLowerCase());
}

/**
 * Fuzzy match of `query` against an entry name, ignoring case and diacritics. Matches against the
 * NFC form: fuzzysort's highlight indexes split NFD accents from their letters.
 */
export function matchName(name: string, query: string): Result | null {
  return single(query, name.normalize('NFC'));
}

/**
 * Entries whose names fuzzy-match `query`, best match first; equal scores keep the input order
 * (the pane's sort). A blank query returns `entries`.
 */
export function filterEntries(entries: readonly Entry[], query: string): readonly Entry[] {
  if (query.trim() === '') return entries;

  const order = new Map(entries.map((e, i) => [e, i]));
  const results = go(query, entries, {
    key: (e) => e.name.normalize('NFC'),
    limit: 0,
    threshold: 0,
  });
  return results
    .toSorted((a, b) => b.score - a.score || (order.get(a.obj) ?? 0) - (order.get(b.obj) ?? 0))
    .map((r) => r.obj);
}

/** Typing pauses longer than this start a new type-ahead prefix. */
export const TYPE_AHEAD_RESET_MS = 1000;

export interface TypeAhead {
  text: string;
  at: number;
}

export const EMPTY_TYPE_AHEAD: TypeAhead = { text: '', at: -Infinity };

export function typeKey(state: TypeAhead, key: string, now: number): TypeAhead {
  const text = now - state.at > TYPE_AHEAD_RESET_MS ? key : state.text + key;
  return { text, at: now };
}

/**
 * Index of the entry type-ahead should land on, or -1: the first name starting with `text`.
 * Repeating one letter ("sss") with no such name steps through names starting with that letter,
 * after `cursor` and wrapping.
 */
export function typeAheadIndex(names: readonly string[], text: string, cursor: number): number {
  const q = foldChars(text);
  const [first] = q;
  if (first === undefined) return -1;
  const folded = names.map(foldChars);

  const hit = folded.findIndex((n) => q.every((c, i) => n[i] === c));
  if (hit >= 0 || q.length === 1 || !q.every((c) => c === first)) return hit;

  const after = folded.findIndex((n, i) => i > cursor && n[0] === first);
  return after >= 0 ? after : folded.findIndex((n) => n[0] === first);
}
