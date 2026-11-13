import { describe, expect, it } from 'vitest';
import { parseQuickConnect } from './quickConnect';

const ok = (user: string, host: string, port: number) => ({
  ok: true,
  target: { user, host, port },
});

describe('parseQuickConnect', () => {
  it('parses user@host:port', () => {
    expect(parseQuickConnect('alice@example.com:2222')).toEqual(ok('alice', 'example.com', 2222));
  });
  it('allows omitting the user and the port', () => {
    expect(parseQuickConnect('example.com')).toEqual(ok('', 'example.com', 0));
    expect(parseQuickConnect('bob@example.com')).toEqual(ok('bob', 'example.com', 0));
    expect(parseQuickConnect('example.com:22')).toEqual(ok('', 'example.com', 22));
  });
  it('strips an ssh:// prefix and surrounding whitespace', () => {
    expect(parseQuickConnect('  ssh://bob@h:1  ')).toEqual(ok('bob', 'h', 1));
  });
  it('splits on the last @', () => {
    expect(parseQuickConnect('a@b@host')).toEqual(ok('a@b', 'host', 0));
  });
  it('handles bracketed IPv6', () => {
    expect(parseQuickConnect('u@[fe80::1]:2200')).toEqual(ok('u', 'fe80::1', 2200));
    expect(parseQuickConnect('[::1]')).toEqual(ok('', '::1', 0));
  });
  it.each([
    '',
    '   ',
    '@host',
    'user@',
    'host:',
    'host:0',
    'host:70000',
    'host:abc',
    'a b@c',
    '::1',
    '[::1',
    '[::1]x',
  ])('rejects %j', (input) => {
    expect(parseQuickConnect(input).ok).toBe(false);
  });
});
