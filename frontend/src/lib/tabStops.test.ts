import { describe, expect, it } from 'vitest';
import { nextTabStop, tabStops } from './tabStops';

function mount(html: string): HTMLElement {
  const root = document.createElement('div');
  root.innerHTML = html;
  document.body.replaceChildren(root);
  return root;
}

const ids = (els: HTMLElement[]) => els.map((el) => el.id);

describe('tabStops', () => {
  it('cycles through dialog buttons in both directions, wrapping', () => {
    const root = mount('<button id="cancel">Cancel</button><button id="delete">Delete</button>');
    const cancel = root.querySelector('#cancel');
    const del = root.querySelector('#delete');

    expect(nextTabStop(root, cancel, false)?.id).toBe('delete');
    expect(nextTabStop(root, del, false)?.id).toBe('cancel');
    expect(nextTabStop(root, cancel, true)?.id).toBe('delete');
    expect(nextTabStop(root, document.body, false)?.id).toBe('cancel');
    expect(nextTabStop(root, document.body, true)?.id).toBe('delete');
  });

  it('skips disabled, hidden, and tabindex=-1 controls', () => {
    const root = mount(`
      <input id="name">
      <button id="off" disabled>Off</button>
      <input type="hidden" id="h">
      <div hidden><button id="gone">Gone</button></div>
      <button id="skip" tabindex="-1">Skip</button>
      <select id="sel"><option>a</option></select>
      <button id="ok">OK</button>`);
    expect(ids(tabStops(root))).toEqual(['name', 'sel', 'ok']);
  });

  it('stops once per radio group, on the checked radio', () => {
    const root = mount(`
      <input type="radio" name="auth" id="profile">
      <input type="radio" name="auth" id="keys" checked>
      <input type="radio" name="other" id="o1">
      <input type="radio" name="other" id="o2">
      <button id="save">Save</button>`);
    expect(ids(tabStops(root))).toEqual(['keys', 'o1', 'save']);
  });

  it('treats an unchecked focused radio as its group stop', () => {
    const root = mount(`
      <input type="radio" name="auth" id="a" checked>
      <input type="radio" name="auth" id="b">
      <button id="save">Save</button>`);
    expect(nextTabStop(root, root.querySelector('#b'), false)?.id).toBe('save');
    expect(nextTabStop(root, root.querySelector('#b'), true)?.id).toBe('save');
  });

  it('returns nothing to focus in an empty dialog', () => {
    expect(nextTabStop(mount('<p>text</p>'), null, false)).toBeUndefined();
  });
});
