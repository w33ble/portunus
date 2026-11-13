import { describe, expect, it } from 'vitest';
import { EMPTY_HISTORY, HISTORY_LIMIT, back, forward, historyKey, visit } from './history';

describe('pane history', () => {
  it('records the folder left behind and walks back and forward through it', () => {
    let h = visit(EMPTY_HISTORY, '/home', '/home/docs');
    h = visit(h, '/home/docs', '/home/docs/a');

    const b1 = back(h, '/home/docs/a');
    expect(b1?.path).toBe('/home/docs');
    const b2 = back(b1!.history, '/home/docs');
    expect(b2?.path).toBe('/home');
    expect(back(b2!.history, '/home')).toBeNull();

    const f1 = forward(b2!.history, '/home');
    expect(f1?.path).toBe('/home/docs');
    const f2 = forward(f1!.history, '/home/docs');
    expect(f2?.path).toBe('/home/docs/a');
    expect(forward(f2!.history, '/home/docs/a')).toBeNull();
  });

  it('drops forward entries when navigating somewhere new', () => {
    let h = visit(EMPTY_HISTORY, '/a', '/b');
    const step = back(h, '/b')!;
    h = visit(step.history, step.path, '/c');
    expect(h.forward).toEqual([]);
    expect(h.back).toEqual(['/a']);
  });

  it('ignores navigation to the same folder (refresh)', () => {
    const h = visit(EMPTY_HISTORY, '/a', '/b');
    expect(visit(h, '/b', '/b')).toBe(h);
  });

  it('keeps at most HISTORY_LIMIT entries, dropping the oldest', () => {
    let h = EMPTY_HISTORY;
    for (let i = 0; i <= HISTORY_LIMIT + 5; i++) h = visit(h, `/d${i}`, `/d${i + 1}`);
    expect(h.back).toHaveLength(HISTORY_LIMIT);
    expect(h.back[0]).toBe('/d6');
    expect(h.back.at(-1)).toBe(`/d${HISTORY_LIMIT + 5}`);
  });
});

const ev = (
  key: string,
  mods: Partial<Record<'metaKey' | 'ctrlKey' | 'altKey' | 'shiftKey', boolean>> = {}
) => ({
  key,
  metaKey: false,
  ctrlKey: false,
  altKey: false,
  shiftKey: false,
  ...mods,
});

describe('historyKey', () => {
  it('maps ⌘[ and ⌘] on macOS', () => {
    expect(historyKey(ev('[', { metaKey: true }), true)).toBe('back');
    expect(historyKey(ev(']', { metaKey: true }), true)).toBe('forward');
    expect(historyKey(ev('[', { metaKey: true, shiftKey: true }), true)).toBeNull();
    expect(historyKey(ev('ArrowLeft', { altKey: true }), true)).toBeNull();
  });

  it('maps Alt+Left and Alt+Right elsewhere', () => {
    expect(historyKey(ev('ArrowLeft', { altKey: true }), false)).toBe('back');
    expect(historyKey(ev('ArrowRight', { altKey: true }), false)).toBe('forward');
    expect(historyKey(ev('ArrowLeft', { altKey: true, ctrlKey: true }), false)).toBeNull();
    expect(historyKey(ev('[', { ctrlKey: true }), false)).toBeNull();
  });
});
