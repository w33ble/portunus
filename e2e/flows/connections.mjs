import { flow, assert, HOME } from './lib.mjs';

const REMOTE = `${process.env.E2E_DIR}/remote`;

// Bookmarks, favourites and per-connection settings from #55, against the real sshd.
await flow('connections', async ({ page, left, right, row, step, shot, goTo }) => {
  const conns = page.locator('dialog[open]').first();
  const picker = (pane) => pane.getByLabel('Connection');
  const saved = () => conns.getByRole('button', { name: 'Save' }).and(page.locator(':disabled')).waitFor();
  const valueIs = async (pane, want) => {
    for (let i = 0; i < 50 && (await picker(pane).inputValue()) !== want; i++) {
      await page.waitForTimeout(200);
    }
    assert((await picker(pane).inputValue()) === want, `${want} selected`);
  };

  await row(left, 'Documents').waitFor();

  step('bookmark a folder on an ssh host with ⌘D and pin it');
  await picker(right).selectOption('ssh:testbox');
  await valueIs(right, 'ssh:testbox');
  await goTo(right, REMOTE);
  await right.locator('[role="option"]').first().click({ timeout: 15000 });
  await page.keyboard.press('Control+D');
  await picker(right).selectOption({ label: 'Manage connections…' });
  await conns.getByRole('button', { name: 'Pin remote' }).click();
  await conns.getByRole('radio', { name: 'Green', exact: true }).check();
  await conns.getByRole('button', { name: 'Save' }).click();
  await saved();
  await shot('connections-dialog');
  await conns.getByRole('button', { name: 'Done' }).click();
  await conns.waitFor({ state: 'detached' });
  assert(await right.getByTitle(/green tag/i).isVisible(), 'pane header shows the colour tag');

  step('a favourite connects the local pane and opens the folder');
  await left.getByRole('button', { name: 'Bookmarks' }).click();
  const menu = page.getByRole('menu');
  await menu.getByRole('menuitem', { name: /remote/ }).first().waitFor();
  await shot('bookmarks-menu');
  await menu.getByRole('menuitem', { name: /remote.*testbox/ }).click();
  await valueIs(left, 'ssh:testbox');
  await left.locator('[role="option"]').first().waitFor({ timeout: 15000 });

  step('open-at folder replaces home on connect');
  await picker(left).selectOption({ label: 'Manage connections…' });
  await conns.getByRole('option', { name: /^Local/ }).click();
  await conns.getByLabel('Open at').fill(`${HOME}/Documents`);
  await conns.getByRole('button', { name: 'Save' }).click();
  await saved();
  await conns.getByRole('button', { name: 'Done' }).click();
  await picker(left).selectOption('local');
  await row(left, 'hello.txt').waitFor({ timeout: 15000 });

  step('clean up: clear the open-at folder, bookmark and colour');
  await picker(left).selectOption({ label: 'Manage connections…' });
  await conns.getByRole('option', { name: /^Local/ }).click();
  await conns.getByLabel('Open at').fill('');
  await conns.getByRole('button', { name: 'Save' }).click();
  await saved();
  await conns.getByRole('option', { name: /testbox/ }).click();
  await conns.getByRole('button', { name: 'Remove bookmark remote' }).click();
  await conns.getByRole('radio', { name: 'None', exact: true }).check();
  await conns.getByRole('button', { name: 'Save' }).click();
  await saved();
  await conns.getByRole('button', { name: 'Done' }).click();
});
