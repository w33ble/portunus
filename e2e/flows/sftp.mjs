import { readFileSync, existsSync, readdirSync } from 'node:fs';
import { flow, assert, sameFile, count, previewFixtures, E2E_DIR, HOME } from './lib.mjs';

const REMOTE = `${E2E_DIR}/remote`;

await flow('sftp', async (ctx) => {
  const { page, left, right, row, step, shot, goTo } = ctx;
  await row(left, 'Documents').dblclick();
  await row(left, 'hello.txt').waitFor();

  step('raise transfer concurrency');
  await page.getByRole('tab', { name: /transfers/i }).click();
  await page.getByLabel('Parallel transfers').fill('4');
  await page.getByLabel('Parallel transfers').press('Enter');
  await page.getByLabel('Per connection').fill('2');
  await page.getByLabel('Per connection').press('Enter');
  await page.getByRole('tab', { name: /browser/i }).click();

  step('connect to testbox and trust host key');
  await right.getByLabel('Connection').selectOption('ssh:testbox');
  const trust = page.getByRole('button', { name: /trust/i });
  await trust.waitFor({ timeout: 15000 });
  await shot('hostkey');
  await trust.click();
  await page.waitForTimeout(1000);
  await goTo(right, REMOTE);
  await row(right, 'remote-only.txt').waitFor({ timeout: 15000 });

  step('F5 copy with a conflict, overwrite all');
  await row(left, 'hello.txt').click();
  await row(left, 'random.bin').click({ modifiers: ['Control'] });
  await row(left, 'photos').click({ modifiers: ['Control'] });
  await row(left, 'pixel.png').click({ modifiers: ['Control'] });
  await row(left, 'doc.pdf').click({ modifiers: ['Control'] });
  await page.keyboard.press('F5');
  await page.getByRole('button', { name: /overwrite/i }).waitFor({ timeout: 15000 });
  await shot('conflict');
  await page.getByRole('button', { name: /overwrite/i }).click();
  await right.getByLabel('Refresh').click();
  await row(right, 'random.bin').waitFor({ timeout: 15000 });
  await row(right, 'photos').waitFor({ timeout: 15000 });

  await page.waitForTimeout(500);
  assert(readFileSync(`${REMOTE}/hello.txt`, 'utf8') === 'hello from portunus\n', 'hello.txt overwritten');
  assert(sameFile(`${HOME}/Documents/random.bin`, `${REMOTE}/random.bin`), 'random.bin identical');
  assert(count(`${REMOTE}/photos/2026`) === 12, '12 photos copied');
  assert(existsSync(`${REMOTE}/photos/empty`), 'empty dir recreated');
  assert(!readdirSync(REMOTE).some((f) => f.endsWith('.portunus-tmp')), 'no temp files left');

  await row(right, 'doc.pdf').waitFor({ timeout: 15000 });
  await previewFixtures(ctx, right, 'sftp');

  step('drag remote → local, then delete it');
  await row(right, 'remote-only.txt').dragTo(left.locator('[data-filelist]'));
  await row(left, 'remote-only.txt').waitFor({ timeout: 15000 });
  assert(existsSync(`${HOME}/Documents/remote-only.txt`), 'dragged file exists locally');
  await row(left, 'remote-only.txt').click();
  await page.keyboard.press('Delete');
  await page.getByRole('button', { name: /^delete$/i }).click();
  await row(left, 'remote-only.txt').waitFor({ state: 'detached', timeout: 15000 });
  assert(!existsSync(`${HOME}/Documents/remote-only.txt`), 'deleted locally');

  step('new folder + rename on remote');
  await right.getByLabel('New folder').click();
  await page.keyboard.type('made-by-e2e');
  await page.keyboard.press('Enter');
  await row(right, 'made-by-e2e').waitFor({ timeout: 15000 });
  await row(right, 'made-by-e2e').click();
  await page.keyboard.press('F2');
  await page.keyboard.press('Control+A');
  await page.keyboard.type('renamed-by-e2e');
  await page.keyboard.press('Enter');
  await row(right, 'renamed-by-e2e').waitFor({ timeout: 15000 });
  assert(existsSync(`${REMOTE}/renamed-by-e2e`), 'renamed dir exists remotely');

  await page.getByRole('tab', { name: /transfers/i }).click();
  await shot('transfers');
  await page.emulateMedia({ colorScheme: 'dark' });
  await page.getByRole('tab', { name: /browser/i }).click();
  await shot('browser-dark');
});
