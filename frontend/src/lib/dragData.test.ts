import { describe, expect, it } from 'vitest';
import { decodeDragPayload, encodeDragPayload, isSameDirectoryDrop } from './dragData';

describe('drag payload', () => {
  it('round-trips', () => {
    const p = { connId: 'local', paths: ['/a/b', '/a/c'] };
    expect(decodeDragPayload(encodeDragPayload(p))).toEqual(p);
  });
  it.each([
    '',
    'nope',
    '[]',
    'null',
    '{"connId":1,"paths":["/a"]}',
    '{"connId":"x","paths":[]}',
    '{"connId":"x","paths":[1]}',
  ])('rejects %j', (raw) => {
    expect(decodeDragPayload(raw)).toBeNull();
  });
});

describe('isSameDirectoryDrop', () => {
  const payload = { connId: 'local', paths: ['/a/x', '/a/y'] };
  it('ignores drops into the source directory', () => {
    expect(isSameDirectoryDrop(payload, { connId: 'local', dir: '/a' })).toBe(true);
  });
  it('ignores dropping a directory onto itself', () => {
    expect(
      isSameDirectoryDrop({ connId: 'local', paths: ['/a/x'] }, { connId: 'local', dir: '/a/x' })
    ).toBe(true);
  });
  it('allows other directories and other connections', () => {
    expect(isSameDirectoryDrop(payload, { connId: 'local', dir: '/b' })).toBe(false);
    expect(isSameDirectoryDrop(payload, { connId: 'ssh:h', dir: '/a' })).toBe(false);
  });
});
