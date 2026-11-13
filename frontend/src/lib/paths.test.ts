import { describe, expect, it } from 'vitest';
import { baseName, breadcrumbs, joinPath, normalizePath, parentPath } from './paths';

describe('normalizePath', () => {
  it('collapses slashes and trims trailing ones', () => {
    expect(normalizePath('/a//b/')).toBe('/a/b');
    expect(normalizePath('  /a/b  ')).toBe('/a/b');
  });
  it('keeps roots', () => {
    expect(normalizePath('/')).toBe('/');
    expect(normalizePath('')).toBe('/');
    expect(normalizePath('C:')).toBe('C:/');
    expect(normalizePath('C:\\Users\\x')).toBe('C:/Users/x');
  });
});

describe('parentPath', () => {
  it('walks up and stops at the root', () => {
    expect(parentPath('/a/b')).toBe('/a');
    expect(parentPath('/a')).toBe('/');
    expect(parentPath('/')).toBe('/');
  });
  it('handles drive roots', () => {
    expect(parentPath('C:/Users')).toBe('C:/');
    expect(parentPath('C:/')).toBe('C:/');
    expect(parentPath('C:/Users/x')).toBe('C:/Users');
  });
});

describe('joinPath / baseName', () => {
  it('joins without doubling slashes', () => {
    expect(joinPath('/', 'a')).toBe('/a');
    expect(joinPath('/a', 'b')).toBe('/a/b');
    expect(joinPath('C:/', 'x')).toBe('C:/x');
  });
  it('returns the last segment', () => {
    expect(baseName('/a/b.txt')).toBe('b.txt');
    expect(baseName('/')).toBe('/');
  });
});

describe('breadcrumbs', () => {
  it('builds cumulative paths', () => {
    expect(breadcrumbs('/a/b')).toEqual([
      { label: '/', path: '/' },
      { label: 'a', path: '/a' },
      { label: 'b', path: '/a/b' },
    ]);
  });
  it('root only', () => {
    expect(breadcrumbs('/')).toEqual([{ label: '/', path: '/' }]);
  });
  it('drive paths', () => {
    expect(breadcrumbs('C:/Users')).toEqual([
      { label: 'C:', path: 'C:/' },
      { label: 'Users', path: 'C:/Users' },
    ]);
  });
});
