import { flow, openPreview } from './lib.mjs';
import { contrastAudit } from './a11y.mjs';

const REMOTE = `${process.env.E2E_DIR}/remote`;

// Opens every dialog in each state a user can reach, and the highlighted quick preview, and checks text contrast in both colour schemes.
await flow('contrast', async ({ page, left, right, row, step, goTo }) => {
  const audit = contrastAudit(page);
  const dialog = page.locator('dialog[open]');
  const close = async () => {
    await page.keyboard.press('Escape');
    await dialog.waitFor({ state: 'detached' });
  };
  const checked = async (label) => {
    await dialog.waitFor({ timeout: 5000 }).catch((e) => {
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
  const connection = right.getByLabel('Connection');
  await connection.selectOption({ label: 'Quick connect…' });
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
  await close();

  step('host key and passphrase prompts');
  await connection.selectOption('ssh:locked');
  await page.getByRole('button', { name: /trust/i }).waitFor({ timeout: 15000 });
  await checked('Host key');
  await page.getByRole('button', { name: /trust/i }).click();
  await dialog.getByLabel(/passphrase/i).waitFor({ timeout: 15000 });
  await checked('Passphrase');
  await page.keyboard.press('Escape');
  await dialog.getByText(/canceled/i).waitFor();
  await close();

  step('conflict');
  await connection.selectOption('local');
  await goTo(left, `${process.env.E2E_DIR}/home/Documents`);
  await goTo(right, REMOTE);
  await row(left, 'hello.txt').click();
  await page.keyboard.press('F5');
  await dialog.getByRole('button', { name: /overwrite/i }).waitFor({ timeout: 15000 });
  await checked('Conflict');
  await dialog.getByRole('button', { name: /cancel job/i }).click();
  await dialog.waitFor({ state: 'detached' });

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

  step('S3 profiles: empty, form, saved, delete confirm');
  await connection.selectOption({ label: 'Manage S3 profiles…' });
  await checked('S3 profiles, empty');
  await dialog.getByRole('button', { name: 'Add profile…' }).click();
  await checked('S3 profile form, AWS profile');
  await dialog.getByLabel('Access key', { exact: true }).check();
  await dialog.getByRole('button', { name: 'Save' }).click();
  await dialog.getByText(/name/i).last().waitFor();
  await checked('S3 profile form, error');
  await dialog.getByLabel('Name', { exact: true }).fill('contrast-check');
  await dialog.getByLabel('Access key ID').fill('key');
  await dialog.getByLabel('Secret access key').fill('secret');
  await checked('S3 profile form, filled');
  await dialog.getByRole('button', { name: 'Save' }).click();
  await dialog.getByText('contrast-check').waitFor();
  await checked('S3 profiles, listed');
  await dialog.getByLabel('Edit contrast-check').click();
  await checked('S3 profile form, editing');
  await dialog.getByRole('button', { name: 'Back' }).click();
  await dialog.getByLabel('Delete contrast-check').click();
  await checked('S3 profiles, delete confirm');
  await dialog.getByRole('button', { name: 'Delete', exact: true }).click();
  await dialog.getByText(/no profiles yet/i).waitFor();
  await close();

  audit.finish();
});
