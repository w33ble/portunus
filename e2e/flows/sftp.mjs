import {
  readFileSync,
  writeFileSync,
  mkdirSync,
  rmSync,
  statSync,
  existsSync,
  readdirSync,
} from 'node:fs';
import { flow, assert, sameFile, count, previewFixtures, E2E_DIR, HOME } from './lib.mjs';

const REMOTE = `${E2E_DIR}/remote`;

await flow('sftp', async (ctx) => {
  const { page, left, right, row, step, shot, goTo, shiftDrag, pick } = ctx;
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
  await pick(right, 'testbox');
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
  await row(left, 'sample.go').click({ modifiers: ['Control'] });
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
  assert(existsSync(`${REMOTE}/remote-only.txt`), 'a plain drag copies and leaves the source');
  await row(left, 'remote-only.txt').click();
  await page.keyboard.press('Delete');
  await page.getByRole('button', { name: /^delete$/i }).click();
  await row(left, 'remote-only.txt').waitFor({ state: 'detached', timeout: 15000 });
  assert(!existsSync(`${HOME}/Documents/remote-only.txt`), 'deleted locally');

  step('F6 moves a file and a folder from local to SFTP');
  const docs = `${HOME}/Documents`;
  writeFileSync(`${docs}/mv-file.txt`, 'moved file\n');
  mkdirSync(`${docs}/mv-dir/nested`, { recursive: true });
  writeFileSync(`${docs}/mv-dir/one.txt`, 'one\n');
  writeFileSync(`${docs}/mv-dir/nested/two.txt`, 'two\n');
  await left.getByLabel('Refresh').click();
  await row(left, 'mv-file.txt').click();
  await row(left, 'mv-dir').click({ modifiers: ['Control'] });
  await page.keyboard.press('F6');
  await row(right, 'mv-dir').waitFor({ timeout: 15000 });
  await row(right, 'mv-file.txt').waitFor({ timeout: 15000 });
  await row(left, 'mv-dir').waitFor({ state: 'detached', timeout: 15000 });
  await row(left, 'mv-file.txt').waitFor({ state: 'detached', timeout: 15000 });
  assert(!existsSync(`${docs}/mv-file.txt`) && !existsSync(`${docs}/mv-dir`), 'local sources are gone');
  assert(readFileSync(`${REMOTE}/mv-file.txt`, 'utf8') === 'moved file\n', 'file arrived on the remote');
  assert(readFileSync(`${REMOTE}/mv-dir/nested/two.txt`, 'utf8') === 'two\n', 'folder arrived on the remote');
  assert(!readdirSync(REMOTE).some((f) => f.endsWith('.portunus-tmp')), 'no temp files after the move');

  step('Shift-drag onto a folder renames within one SFTP connection');
  const inode = statSync(`${REMOTE}/mv-file.txt`).ino;
  await shiftDrag(row(right, 'mv-file.txt'), row(right, 'mv-dir').locator('[data-file-drop-target]'));
  await row(right, 'mv-file.txt').waitFor({ state: 'detached', timeout: 15000 });
  assert(!existsSync(`${REMOTE}/mv-file.txt`), 'source left the remote directory');
  assert(existsSync(`${REMOTE}/mv-dir/mv-file.txt`), 'file is inside the folder');
  assert(statSync(`${REMOTE}/mv-dir/mv-file.txt`).ino === inode, 'same connection renames, no data copied');

  step('Shift-drag SFTP to local moves, F6 moves a folder back');
  await row(right, 'mv-dir').dblclick();
  await row(right, 'mv-file.txt').waitFor({ timeout: 15000 });
  await shiftDrag(row(right, 'mv-file.txt'), left.locator('[data-filelist]'));
  await row(left, 'mv-file.txt').waitFor({ timeout: 15000 });
  assert(readFileSync(`${docs}/mv-file.txt`, 'utf8') === 'moved file\n', 'dragged file arrived locally');
  assert(!existsSync(`${REMOTE}/mv-dir/mv-file.txt`), 'dragged file left the remote');
  await right.getByLabel('Up one level').click();
  await row(right, 'mv-dir').click();
  await page.keyboard.press('F6');
  await row(left, 'mv-dir').waitFor({ timeout: 15000 });
  await row(right, 'mv-dir').waitFor({ state: 'detached', timeout: 15000 });
  assert(readFileSync(`${docs}/mv-dir/nested/two.txt`, 'utf8') === 'two\n', 'folder arrived locally');
  assert(!existsSync(`${REMOTE}/mv-dir`), 'remote folder is gone');

  step('a skipped conflict stays at the source');
  writeFileSync(`${docs}/mv-conflict.txt`, 'local copy\n');
  writeFileSync(`${REMOTE}/mv-conflict.txt`, 'remote copy\n');
  await left.getByLabel('Refresh').click();
  await row(left, 'mv-conflict.txt').click();
  await page.keyboard.press('F6');
  await page.getByRole('button', { name: /skip/i }).click();
  await page.waitForTimeout(1000);
  assert(readFileSync(`${docs}/mv-conflict.txt`, 'utf8') === 'local copy\n', 'skipped source kept');
  assert(readFileSync(`${REMOTE}/mv-conflict.txt`, 'utf8') === 'remote copy\n', 'target untouched');

  rmSync(`${docs}/mv-file.txt`);
  rmSync(`${docs}/mv-dir`, { recursive: true });
  rmSync(`${docs}/mv-conflict.txt`);
  rmSync(`${REMOTE}/mv-conflict.txt`);
  await left.getByLabel('Refresh').click();
  await right.getByLabel('Refresh').click();

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
