import { flow, assert, sameFile, previewFixtures, HOME } from './lib.mjs';

await flow('s3', async (ctx) => {
  const { page, left, right, row, step, shot, goTo } = ctx;
  await goTo(left, `${HOME}/Documents`);
  await row(left, 'random.bin').waitFor();

  step('create S3 profile');
  await right.getByLabel('Connection').selectOption({ label: 'Manage S3 profiles…' });
  await page.getByRole('button', { name: 'Add profile…' }).click();
  const dlg = page.getByRole('dialog');
  await dlg.getByLabel('Name', { exact: true }).fill('rustfs-e2e');
  await dlg.getByLabel('Access key', { exact: true }).check();
  await dlg.getByLabel('Access key ID').fill(process.env.S3_USER);
  await dlg.getByLabel('Secret access key').fill(process.env.S3_PASS);
  await dlg.getByLabel('Endpoint (optional)').fill(process.env.S3_ENDPOINT);
  await dlg.getByLabel(/path-style/).check();
  await shot('profile-form');
  await page.getByRole('button', { name: 'Save' }).click();
  await page.getByRole('button', { name: 'Done' }).click();

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
  for (const name of ['hello.txt', 'pixel.png', 'doc.pdf']) {
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
});
