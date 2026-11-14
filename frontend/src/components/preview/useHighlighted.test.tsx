import { act } from 'react';
import { type Root, createRoot } from 'react-dom/client';
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { HIGHLIGHT_LIMIT_BYTES, type Language } from '../../lib/syntax';
import { deferred } from '../../stores/testApi';
import { highlight } from './highlighter';
import { useHighlighted } from './useHighlighted';

vi.mock('./highlighter', () => ({ highlight: vi.fn() }));

function Probe({ name, text, bytes }: { name: string; text: string; bytes: number }) {
  return <output>{useHighlighted(name, text, bytes) ?? 'plain'}</output>;
}

let root: Root;
let host: HTMLElement;

beforeAll(() => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
});

beforeEach(() => {
  vi.mocked(highlight).mockReset();
  host = document.createElement('div');
  root = createRoot(host);
});

afterEach(() => act(() => root.unmount()));

const show = (name: string, text: string, bytes = text.length) =>
  act(async () => root.render(<Probe name={name} text={text} bytes={bytes} />));
const shown = () => host.querySelector('output')?.textContent;

describe('useHighlighted', () => {
  it('is plain until the highlight arrives, then shows its markup', async () => {
    const pending = deferred<string>();
    vi.mocked(highlight).mockReturnValue(pending.promise);
    await show('main.go', 'package main');
    expect(shown()).toBe('plain');
    expect(highlight).toHaveBeenCalledWith('go' satisfies Language, 'package main');

    await act(async () => pending.resolve('<span>package</span> main'));
    expect(shown()).toBe('<span>package</span> main');
  });

  it('drops the highlight of a file the preview already moved away from', async () => {
    const first = deferred<string>();
    const second = deferred<string>();
    vi.mocked(highlight).mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    await show('a.go', 'package a');
    await show('b.py', 'import b');

    await act(async () => first.resolve('FIRST'));
    expect(shown()).toBe('plain');
    await act(async () => second.resolve('SECOND'));
    expect(shown()).toBe('SECOND');
  });

  it('shows plain text for a file with no language without highlighting', async () => {
    await show('notes.txt', 'package main');
    expect(shown()).toBe('plain');
    expect(highlight).not.toHaveBeenCalled();
  });

  it('shows plain text for a file over the size cap without highlighting', async () => {
    await show('big.go', 'package main', HIGHLIGHT_LIMIT_BYTES + 1);
    expect(shown()).toBe('plain');
    expect(highlight).not.toHaveBeenCalled();
  });

  it('stays plain, without an error, when the grammar fails to load', async () => {
    vi.mocked(highlight).mockRejectedValue(new Error('chunk failed'));
    await show('main.go', 'package main');
    expect(shown()).toBe('plain');
  });
});
