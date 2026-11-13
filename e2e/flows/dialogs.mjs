import { writeFileSync, existsSync } from 'node:fs';
import { flow, assert, HOME } from './lib.mjs';

const VICTIM = `${HOME}/Documents/tab-victim.txt`;

await flow(
  'dialogs',
  async ({ page, left, row, step, shot }) => {
    writeFileSync(VICTIM, 'delete me\n');
    await row(left, 'Documents').dblclick();
    await row(left, 'tab-victim.txt').waitFor({ timeout: 15000 });

    const inDialog = () => page.evaluate(() => !!document.activeElement?.closest('dialog[open]'));
    const focused = () => page.evaluate(() => document.activeElement?.textContent?.trim());

    step('delete dialog: Tab reaches Delete, wraps both ways, stays inside');
    await row(left, 'tab-victim.txt').click();
    await page.keyboard.press('Delete');
    await page.getByRole('button', { name: /^delete$/i }).waitFor();
    assert((await focused()) === 'Cancel', 'initial focus on Cancel');

    await page.keyboard.press('Tab');
    assert((await focused()) === 'Delete', 'Tab from Cancel lands on Delete');
    await page.keyboard.press('Tab');
    assert((await focused()) === 'Cancel', 'Tab from last control wraps to first');
    await page.keyboard.press('Shift+Tab');
    assert((await focused()) === 'Delete', 'Shift+Tab from first control wraps to last');
    for (let i = 0; i < 5; i++) {
      await page.keyboard.press('Tab');
      assert(await inDialog(), 'focus stays inside the dialog');
    }
    await shot('delete-focus');

    step('Enter on Delete deletes');
    await page.keyboard.press('Tab');
    assert((await focused()) === 'Delete', 'focus on Delete');
    await page.keyboard.press('Enter');
    await page.waitForFunction(() => !document.querySelector('dialog[open]'));
    await page.waitForTimeout(500);
    assert(!existsSync(VICTIM), 'file deleted');
  },
  { engine: 'webkit' }
);
