import { readFileSync, writeFileSync, mkdirSync, existsSync } from 'node:fs';
import { flow, assert, sameFile, previewFixtures, HOME } from './lib.mjs';

await flow('s3', async (ctx) => {
  const { page, left, right, row, step, shot, goTo, shiftDrag } = ctx;
  await goTo(left, `${HOME}/Documents`);
  await row(left, 'random.bin').waitFor();

  step('create S3 profile');
  await right.getByLabel('Connection').selectOption({ label: 'Manage connections…' });
  const dlg = page.getByRole('dialog');
  await dlg.getByRole('button', { name: 'Add connection' }).click();
  await dlg.getByRole('button', { name: 'S3 profile' }).click();
  await dlg.getByLabel('Provider').selectOption({ label: 'MinIO / RustFS (local)' });
  await dlg.getByLabel('Name', { exact: true }).fill('rustfs-e2e');
  const accessKey = dlg.getByLabel('Access key', { exact: true });
  assert(await accessKey.isChecked(), 'preset picks access keys');
  assert(await dlg.getByLabel(/path-style/).isChecked(), 'preset picks path-style');
  await dlg.getByLabel('Access key ID').fill(process.env.S3_USER);
  await dlg.getByLabel('Secret access key').fill(process.env.S3_PASS);
  await dlg.getByLabel('Endpoint', { exact: true }).fill(process.env.S3_ENDPOINT);
  await shot('profile-form');
  await dlg.getByRole('button', { name: 'Save' }).click();
  await dlg.getByRole('option', { name: /^rustfs-e2e/ }).waitFor();
  await dlg.getByRole('button', { name: 'Done' }).click();

  step('connect and create a bucket');
  await right.getByLabel('Connection').selectOption({ label: 'rustfs-e2e' });
  await page.waitForTimeout(1000);
  await right.getByLabel('New folder').click();
  await page.keyboard.type('e2e-bucket');
  await page.keyboard.press('Enter');
  await row(right, 'e2e-bucket').waitFor({ timeout: 15000 });
  await row(right, 'e2e-bucket').dblclick();
  await page.waitForTimeout(800);

  step('upload a file and a tree');
  await row(left, 'random.bin').click();
  await row(left, 'photos').click({ modifiers: ['Control'] });
  for (const name of ['hello.txt', 'pixel.png', 'doc.pdf', 'sample.go']) {
    await row(left, name).click({ modifiers: ['Control'] });
  }
  await page.keyboard.press('F5');
  await row(right, 'random.bin').waitFor({ timeout: 20000 });
  await row(right, 'photos').waitFor({ timeout: 20000 });
  await row(right, 'doc.pdf').waitFor({ timeout: 20000 });

  await previewFixtures(ctx, right, 's3');

  step('download back and compare');
  await left.getByLabel('New folder').click();
  await page.keyboard.type('download');
  await page.keyboard.press('Enter');
  await row(left, 'download').waitFor({ timeout: 15000 });
  await row(left, 'download').dblclick();
  await page.waitForTimeout(800);
  await row(right, 'random.bin').dragTo(left.locator('[data-filelist]'));
  await row(left, 'random.bin').waitFor({ timeout: 20000 });
  await page.waitForTimeout(300);
  assert(sameFile(`${HOME}/Documents/random.bin`, `${HOME}/Documents/download/random.bin`), 'S3 round trip identical');

  step('browse virtual dirs incl. empty marker');
  await row(right, 'photos').dblclick();
  await row(right, '2026').waitFor({ timeout: 15000 });
  await row(right, 'empty').waitFor({ timeout: 15000 });
  await shot('s3-browse');

  step('move between local and S3');
  const dl = `${HOME}/Documents/download`;
  writeFileSync(`${dl}/mv-s3.txt`, 's3 moved\n');
  mkdirSync(`${dl}/mv-s3-dir/sub`, { recursive: true });
  writeFileSync(`${dl}/mv-s3-dir/sub/inner.txt`, 'inner\n');
  await left.getByLabel('Refresh').click();
  await right.getByLabel('Up one level').click();
  await row(right, 'random.bin').waitFor({ timeout: 15000 });
  await row(left, 'mv-s3.txt').click();
  await row(left, 'mv-s3-dir').click({ modifiers: ['Control'] });
  await page.keyboard.press('F6');
  await row(right, 'mv-s3-dir').waitFor({ timeout: 20000 });
  await row(right, 'mv-s3.txt').waitFor({ timeout: 20000 });
  await row(left, 'mv-s3-dir').waitFor({ state: 'detached', timeout: 15000 });
  await row(left, 'mv-s3.txt').waitFor({ state: 'detached', timeout: 15000 });
  assert(!existsSync(`${dl}/mv-s3.txt`) && !existsSync(`${dl}/mv-s3-dir`), 'local sources are gone');

  step('Shift-drag moves an object, then a prefix, within S3');
  await shiftDrag(row(right, 'mv-s3.txt'), row(right, 'mv-s3-dir').locator('[data-file-drop-target]'));
  await row(right, 'mv-s3.txt').waitFor({ state: 'detached', timeout: 20000 });
  await right.getByLabel('New folder').click();
  await page.keyboard.type('mv-s3-target');
  await page.keyboard.press('Enter');
  await row(right, 'mv-s3-target').waitFor({ timeout: 15000 });
  await shiftDrag(row(right, 'mv-s3-dir'), row(right, 'mv-s3-target').locator('[data-file-drop-target]'));
  await row(right, 'mv-s3-dir').waitFor({ state: 'detached', timeout: 20000 });

  step('F6 moves the prefix from S3 to local');
  await row(right, 'mv-s3-target').click();
  await page.keyboard.press('F6');
  await row(left, 'mv-s3-target').waitFor({ timeout: 20000 });
  await row(right, 'mv-s3-target').waitFor({ state: 'detached', timeout: 20000 });
  assert(readFileSync(`${dl}/mv-s3-target/mv-s3-dir/mv-s3.txt`, 'utf8') === 's3 moved\n', 'object arrived locally');
  assert(readFileSync(`${dl}/mv-s3-target/mv-s3-dir/sub/inner.txt`, 'utf8') === 'inner\n', 'nested object arrived locally');
});
