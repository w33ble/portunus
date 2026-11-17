import { flow, openPreview } from './lib.mjs';
import { contrastAudit } from './a11y.mjs';

const REMOTE = `${process.env.E2E_DIR}/remote`;

// Opens every dialog in each state a user can reach, and the highlighted quick preview, and checks text contrast in every theme.
await flow('contrast', async ({ page, left, right, row, step, goTo, pick }) => {
  const audit = contrastAudit(page);
  const dialog = page.locator('dialog[open]');
  const close = async () => {
    await page.keyboard.press('Escape');
    await dialog.waitFor({ state: 'detached' });
  };
  const checked = async (label) => {
    await dialog
      .first()
      .waitFor({ timeout: 5000 })
      .catch((e) => {
        throw new Error(`${label}: dialog never opened\n${e.message}`);
      });
    await audit.check(label);
  };

  await row(left, 'Documents').waitFor();

  step('new folder: empty (disabled Create), then a name that already exists');
  await right.getByLabel('New folder').click();
  await checked('New folder, empty');
  await page.keyboard.type('Documents');
  await page.keyboard.press('Enter');
  await dialog.getByText(/exist/i).waitFor();
  await checked('New folder, error');
  await close();

  step('rename and delete');
  await row(left, 'Documents').click();
  await page.keyboard.press('F2');
  await checked('Rename');
  await close();
  await row(left, 'Documents').click();
  await page.keyboard.press('Delete');
  await checked('Delete one folder');
  await close();
  await row(left, 'Documents').dblclick();
  await row(left, 'hello.txt').click();
  await row(left, 'random.bin').click({ modifiers: ['Control'] });
  await page.keyboard.press('Delete');
  await checked('Delete several');
  await close();

  step('quick connect: empty, invalid, refused');
  await pick(right, 'Quick connect…');
  await checked('Quick connect, empty');
  await page.keyboard.type('a@');
  await page.keyboard.press('Enter');
  await dialog.getByText(/host is empty/i).waitFor();
  await checked('Quick connect, error');
  await page.keyboard.press('Control+A');
  await page.keyboard.type('127.0.0.1:1');
  await page.keyboard.press('Enter');
  await dialog.getByText(/connect/i).first().waitFor({ timeout: 15000 });
  await checked('Error dialog');
  await close();

  step('settings');
  await page.getByLabel('Settings').click();
  await dialog.getByLabel('Parallel transfers').waitFor();
  await checked('Settings');
  await dialog.getByRole('tab', { name: 'Appearance' }).click();
  await dialog.getByRole('radio', { name: /Nord Dark/ }).waitFor();
  await checked('Settings, appearance');
  await close();

  step('host key and passphrase prompts');
  await pick(right, 'locked');
  await page.getByRole('button', { name: /trust/i }).waitFor({ timeout: 15000 });
  await checked('Host key');
  await page.getByRole('button', { name: /trust/i }).click();
  await dialog.getByLabel(/passphrase/i).waitFor({ timeout: 15000 });
  await checked('Passphrase');
  await page.keyboard.press('Escape');
  await dialog.getByText(/canceled/i).waitFor();
  await close();

  step('conflict');
  await pick(right, 'Local');
  await goTo(left, `${process.env.E2E_DIR}/home/Documents`);
  await goTo(right, REMOTE);
  await row(left, 'hello.txt').click();
  await page.keyboard.press('F5');
  await dialog.getByRole('button', { name: /overwrite/i }).waitFor({ timeout: 15000 });
  await checked('Conflict');
  await dialog.getByRole('button', { name: /cancel job/i }).click();
  await dialog.waitFor({ state: 'detached' });

  step('main window: a selection in the browser, then the canceled job in transfers');
  await row(left, 'hello.txt').click();
  await audit.check('Browser', { scope: '#root' });
  await page.getByRole('tab', { name: /transfers/i }).click();
  await page.getByText(/canceled/i).first().waitFor({ timeout: 15000 });
  await audit.check('Transfers', { scope: '#root' });
  await page.getByRole('tab', { name: /browser/i }).click();

  step('quick preview: highlighted code and a diff');
  const panel = page.locator('section[data-preview]');
  const previewed = async (name, token, label) => {
    await openPreview({ page, row }, left, name);
    await panel.locator(`pre ${token}`).first().waitFor({ timeout: 15000 });
    // Every token class counts: axe samples text, so assert on the custom measurement too.
    await audit.check(label, { scope: 'section[data-preview]', strictText: true });
  };
  await previewed('sample.go', '.hljs-keyword', 'Preview, Go');
  await previewed('sample.patch', '.hljs-addition', 'Preview, diff');
  await previewed('Dockerfile', '.hljs-keyword', 'Preview, Dockerfile');
  await page.keyboard.press('Escape');
  await panel.waitFor({ state: 'detached' });

  step('bookmarks menu');
  await page.getByRole('tab', { name: /browser/i }).click();
  await row(left, 'hello.txt').click();
  await page.keyboard.press('Control+D');
  await left.getByRole('button', { name: 'Bookmarks' }).click();
  await page.getByText('Remove bookmark…').waitFor();
  await audit.check('Bookmarks menu', { scope: '#root' });
  await page.keyboard.press('Escape');

  step('connections: local, config host, hidden host, S3 form, unsaved prompt, remove confirm');
  const conns = dialog.first();
  const option = (name) => conns.getByRole('option', { name });
  await pick(right, 'Manage connections…');
  await option(/^Local/).click();
  await conns.getByRole('textbox', { name: /Name for/ }).first().waitFor();
  await checked('Connections, local with a bookmark');
  await conns.getByLabel('Open at').selectOption({ index: 1 });
  await conns.getByRole('img', { name: 'Opens here' }).waitFor();
  await checked('Connections, open-at bookmark');
  await conns.getByRole('button', { name: 'Revert', exact: true }).click();
  await conns.getByRole('button', { name: /^Remove bookmark/ }).first().click();
  await conns.getByRole('button', { name: 'Keep', exact: true }).waitFor();
  await checked('Connections, bookmark remove confirm');
  await conns.getByRole('button', { name: 'Keep', exact: true }).click();
  await option(/^locked/).click();
  await conns.getByText(/ProxyJump|@/).first().waitFor({ timeout: 15000 });
  await checked('Connections, config host');
  await conns.getByRole('button', { name: 'Hide from picker' }).click();
  await conns.getByRole('button', { name: 'Favorite', exact: true }).click();
  await checked('Connections, config host edited');
  await conns.getByRole('button', { name: 'Save' }).click();
  await conns.getByRole('button', { name: 'Save' }).and(page.locator(':disabled')).waitFor();
  await checked('Connections, hidden favorite host');
  await close();

  step('connection picker: list, filter, hidden shown');
  await right.getByRole('button', { name: 'Connection', exact: true }).click();
  const picker = page.getByRole('listbox', { name: 'Connect to' });
  await picker.waitFor();
  await audit.check('Connection picker', { scope: '#root' });
  await page.getByRole('button', { name: /hidden · Show/ }).click();
  await picker.getByRole('option', { name: 'locked', exact: true }).waitFor();
  await audit.check('Connection picker, hidden shown', { scope: '#root' });
  await page.keyboard.type('zzz');
  await page.getByText('No matching connections').waitFor();
  await audit.check('Connection picker, no match', { scope: '#root' });
  await page.keyboard.press('Escape');
  await picker.waitFor({ state: 'detached' });

  step('connections: undo hide, new host, S3 form, unsaved prompt, remove confirm');
  await pick(right, 'Manage connections…');
  await option(/locked/).click();
  await conns.getByRole('button', { name: 'Hide from picker' }).click();
  await conns.getByRole('button', { name: 'Favorite', exact: true }).click();
  await conns.getByRole('button', { name: 'Save' }).click();
  await conns.getByRole('button', { name: 'Save' }).and(page.locator(':disabled')).waitFor();

  await conns.getByRole('button', { name: 'Add connection' }).click();
  await conns.getByRole('button', { name: 'SSH host' }).click();
  await checked('Connections, new SSH host');
  await conns.getByRole('button', { name: 'Remove connection' }).click();
  await conns.getByRole('button', { name: 'Add connection' }).click();
  await conns.getByRole('button', { name: 'S3 profile' }).click();
  await conns.getByLabel('Access key', { exact: true }).check();
  await conns.getByRole('button', { name: 'Save' }).click();
  await conns.getByRole('alert').waitFor();
  await checked('Connections, S3 form error');
  await conns.getByLabel('Provider').selectOption({ label: 'Backblaze B2' });
  await checked('Connections, S3 Backblaze B2 preset');
  await conns.getByLabel('Provider').selectOption({ label: 'Amazon S3' });
  await conns.getByLabel('Name', { exact: true }).fill('contrast-check');
  await conns.getByLabel('Access key ID').fill('key');
  await conns.getByLabel('Secret access key').fill('secret');
  await conns.getByRole('button', { name: 'Save' }).click();
  await option(/^contrast-check/).waitFor();
  await checked('Connections, S3 profile saved');

  await conns.getByRole('radio', { name: 'Red', exact: true }).click();
  await option(/^Local/).click();
  await dialog.last().getByRole('button', { name: 'Don’t save' }).waitFor();
  await checked('Connections, unsaved prompt');
  await dialog.last().getByRole('button', { name: 'Don’t save' }).click();
  await option(/^contrast-check/).click();
  await conns.getByRole('button', { name: 'Remove connection' }).click();
  await checked('Connections, remove confirm');
  await conns.getByRole('button', { name: 'Remove', exact: true }).click();
  await option(/^contrast-check/).waitFor({ state: 'detached' });
  await close();
  await row(left, 'hello.txt').click();
  await page.keyboard.press('Control+D');
  await dialog.getByRole('button', { name: 'Remove', exact: true }).waitFor();
  await checked('Remove bookmark');
  await dialog.getByRole('button', { name: 'Remove', exact: true }).click();
  await dialog.waitFor({ state: 'detached' });

  audit.finish();
});
