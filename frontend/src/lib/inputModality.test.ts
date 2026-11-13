import { afterEach, describe, expect, it } from 'vitest';
import { trackInputModality } from './inputModality';

let stop: (() => void) | undefined;

afterEach(() => stop?.());

const key = (k: string) => window.dispatchEvent(new KeyboardEvent('keydown', { key: k }));
const pointer = () => window.dispatchEvent(new Event('pointerdown'));

describe('trackInputModality', () => {
  it('starts as pointer so launch focus draws no ring', () => {
    const root = document.createElement('div');
    stop = trackInputModality(root);
    expect(root.dataset['modality']).toBe('pointer');
  });

  it('switches to keyboard on navigation keys and back on pointer presses', () => {
    const root = document.createElement('div');
    stop = trackInputModality(root);

    key('ArrowDown');
    expect(root.dataset['modality']).toBe('keyboard');
    pointer();
    expect(root.dataset['modality']).toBe('pointer');
    key('Tab');
    expect(root.dataset['modality']).toBe('keyboard');
  });

  it('ignores non-navigation keys such as shortcuts', () => {
    const root = document.createElement('div');
    stop = trackInputModality(root);

    key('a');
    key('Meta');
    key('F5');
    expect(root.dataset['modality']).toBe('pointer');
  });

  it('stops tracking after cleanup', () => {
    const root = document.createElement('div');
    trackInputModality(root)();
    key('ArrowDown');
    expect(root.dataset['modality']).toBe('pointer');
  });
});
