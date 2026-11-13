import { parentPath } from './paths';

export const ENTRIES_MIME = 'application/x-portunus-entries';

export interface DragPayload {
  connId: string;
  paths: string[];
}

export function encodeDragPayload(payload: DragPayload): string {
  return JSON.stringify(payload);
}

/** Parses untrusted drag data; anything malformed yields null. */
export function decodeDragPayload(raw: string): DragPayload | null {
  let value: unknown;
  try {
    value = JSON.parse(raw);
  } catch {
    return null;
  }
  if (typeof value !== 'object' || value === null) return null;
  const connId: unknown = 'connId' in value ? value.connId : undefined;
  const paths: unknown = 'paths' in value ? value.paths : undefined;
  if (typeof connId !== 'string' || !Array.isArray(paths)) return null;
  if (paths.length === 0 || !paths.every((p): p is string => typeof p === 'string')) return null;
  return { connId, paths };
}

/** A copy into the directory the entries already live in is meaningless. */
export function isSameDirectoryDrop(
  payload: DragPayload,
  target: { connId: string; dir: string }
): boolean {
  if (payload.connId !== target.connId) return false;
  return (
    payload.paths.every((p) => parentPath(p) === target.dir) || payload.paths.includes(target.dir)
  );
}
