import { appendFileSync, readFileSync, writeFileSync } from 'node:fs';
import { flow, assert, HOME } from './lib.mjs';

const REMOTE = `${process.env.E2E_DIR}/remote`;
const DOCS = `${HOME}/Documents`;
const SSH_CONFIG = `${HOME}/.ssh/config`;

// Bookmarks, favorites and per-connection settings from #55, against the real sshd.
await flow('connections', async ({ page, left, right, row, step, shot, goTo, pick, current }) => {
  const conns = page.locator('dialog[open]').first();
  const button = (name) => conns.getByRole('button', { name, exact: true });
  const save = async () => {
    await button('Save').click();
    await button('Save').and(page.locator(':disabled')).waitFor();
  };
  const showing = async (pane, want) => {
    for (let i = 0; i < 50 && !(await current(pane)).includes(want); i++) {
      await page.waitForTimeout(200);
    }
    assert((await current(pane)).includes(want), `picker shows ${want}`);
  };

  await row(left, 'Documents').waitFor();

  step('bookmark a folder on an ssh host with ⌘D, pin it, favorite and color the host');
  await pick(right, 'testbox');
  await showing(right, 'testbox');
  await goTo(right, REMOTE);
  await right.locator('[role="option"]').first().click({ timeout: 15000 });
  await page.keyboard.press('Control+D');
  await pick(right, 'Manage connections…');
  await button('Pin remote').click();
  await conns.getByRole('radio', { name: 'Green', exact: true }).check();
  await button('Favorite').click();
  await save();
  await shot('connections-dialog');
  await button('Done').click();
  await conns.waitFor({ state: 'detached' });
  assert(await right.getByTitle(/green tag/i).first().isVisible(), 'picker shows the color tag');

  step('the picker lists favorites first');
  await left.getByRole('button', { name: 'Connection', exact: true }).click();
  const list = page.getByRole('listbox', { name: 'Connect to' });
  await list.getByRole('group', { name: 'Favorites' }).getByRole('option', { name: 'testbox' }).waitFor();
  await shot('connection-picker');
  await page.keyboard.type('test');
  await page.keyboard.press('Enter');
  await showing(left, 'testbox');
  await pick(left, 'Local');
  await showing(left, 'Local');

  step('a favorite bookmark connects the local pane and opens the folder');
  await left.getByRole('button', { name: 'Bookmarks' }).click();
  const menu = page.getByRole('menu');
  await menu.getByRole('menuitem', { name: /remote/ }).first().waitFor();
  await shot('bookmarks-menu');
  await menu.getByRole('menuitem', { name: /remote.*testbox/ }).click();
  await showing(left, 'testbox');
  await left.locator('[role="option"]').first().waitFor({ timeout: 15000 });

  step('open at a bookmarked folder instead of home');
  await pick(left, 'Manage connections…');
  await conns.getByRole('option', { name: /^Local/ }).click();
  await conns.getByLabel('Folder to bookmark').fill(DOCS);
  await button('Add').click();
  await conns.getByLabel('Open at').selectOption(DOCS);
  await conns.getByRole('img', { name: 'Opens here' }).waitFor();
  await save();
  await shot('open-at');
  await button('Done').click();
  await pick(left, 'Local');
  await row(left, 'hello.txt').waitFor({ timeout: 15000 });

  step('bookmark from the context menu, then reorder in the manager');
  const menu2 = page.getByRole('menu');
  const fileList = left.locator('[data-filelist]');
  const box = await fileList.boundingBox();
  await fileList.click({ button: 'right', position: { x: box.width / 2, y: box.height - 20 } });
  await menu2.getByRole('menuitem', { name: /Remove bookmark…/ }).waitFor();
  await page.keyboard.press('Escape');
  await row(left, 'photos').click({ button: 'right' });
  await menu2.getByRole('menuitem', { name: /Bookmark folder/ }).waitFor();
  await shot('context-menu-bookmark', { settle: false });
  await menu2.getByRole('menuitem', { name: /Bookmark folder/ }).click();
  await pick(left, 'Manage connections…');
  await conns.getByRole('option', { name: /^Local/ }).click();
  await button('Move photos').focus();
  await page.keyboard.press('ArrowUp');
  await save();
  await shot('bookmarks-reordered');
  await button('Done').click();
  await left.getByRole('button', { name: 'Bookmarks' }).click();
  const onLocal = await menu2.getByRole('menuitem').allInnerTexts();
  const at = (name) => onLocal.findIndex((t) => t.startsWith(name));
  assert(at('photos') !== -1 && at('photos') < at('Documents'), `bookmark order ${onLocal}`);
  await page.keyboard.press('Escape');

  step('clean up: remove the bookmarks, color and favorite');
  await pick(left, 'Manage connections…');
  await conns.getByRole('option', { name: /^Local/ }).click();
  await button('Remove bookmark Documents').click();
  await button('Remove').click();
  await button('Remove bookmark photos').click();
  await button('Remove').click();
  assert((await conns.getByLabel('Open at').inputValue()) === '', 'open-at falls back to home');
  await save();
  await conns.getByRole('option', { name: /testbox/ }).click();
  await button('Remove bookmark remote').click();
  await button('Remove').click();
  await conns.getByRole('radio', { name: 'None', exact: true }).check();
  await button('Favorite').click();
  await save();
  await button('Done').click();

  step('reload ~/.ssh/config from the manager');
  const original = readFileSync(SSH_CONFIG, 'utf8');
  appendFileSync(SSH_CONFIG, '\nHost added-later\n  HostName 127.0.0.1\n');
  await pick(left, 'Manage connections…');
  assert(!(await conns.getByRole('option', { name: /added-later/ }).count()), 'not listed yet');
  await button('Reload SSH config').click();
  await conns.getByRole('option', { name: /added-later/ }).click();
  await conns.getByText('127.0.0.1').first().waitFor({ timeout: 15000 });
  await shot('reloaded-ssh-config');
  writeFileSync(SSH_CONFIG, original);
  await button('Reload SSH config').click();
  await conns.getByRole('option', { name: /added-later/ }).waitFor({ state: 'detached' });
  const sidebar = conns.getByRole('listbox', { name: 'Connections' });
  assert(
    (await sidebar.getByRole('option', { selected: true }).innerText()).trim() === 'Local',
    'a removed alias falls back to Local'
  );
  await button('Done').click();
});
