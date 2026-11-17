import { assert, flow } from './lib.mjs';

// Theme selection: System follows the OS live, a picked theme applies at once, persists, and is on
// the page before the app's own script runs. Ends back on System for any later flow.
await flow(
  'appearance',
  async ({ page, step, shot }) => {
    const dialog = page.locator('dialog[open]');
    const applied = (theme) =>
      page.waitForFunction((t) => document.documentElement.dataset.theme === t, theme, { timeout: 5000 });
    const openAppearance = async () => {
      await page.getByLabel('Settings').click();
      await dialog.getByRole('tab', { name: 'Appearance' }).click();
    };

    step('System follows the OS appearance live');
    await page.getByLabel('Settings').waitFor();
    await page.emulateMedia({ colorScheme: 'dark' });
    await applied('dark');
    await page.emulateMedia({ colorScheme: 'light' });
    await applied('light');

    step('a picked theme applies at once and ignores the OS');
    await openAppearance();
    await shot('picker-system');
    await dialog.getByText('Nord Dark', { exact: true }).click();
    await applied('nord-dark');
    await page.emulateMedia({ colorScheme: 'dark' });
    await page.emulateMedia({ colorScheme: 'light' });
    await applied('nord-dark');
    await shot('picker-nord-dark');
    await page.keyboard.press('Escape');
    await dialog.waitFor({ state: 'detached' });

    step('the saved theme is painted before the app script runs');
    await page.route('**/assets/*.js', (route) => route.abort());
    await page.reload();
    await applied('nord-dark');
    const bg = await page.evaluate(() => getComputedStyle(document.body).backgroundColor);
    assert(bg === 'rgb(46, 52, 64)', `body background before the app loads = ${bg}, want Nord's #2e3440`);
    await page.unroute('**/assets/*.js');

    step('the choice survives a reload');
    await page.reload();
    await openAppearance();
    assert(await dialog.getByRole('radio', { name: 'Nord Dark' }).isChecked(), 'Nord Dark still selected');
    await shot('reloaded');

    step('System hands control back to the OS');
    await dialog.getByRole('radio', { name: /^System/ }).check({ force: true });
    await applied('light');
    await page.emulateMedia({ colorScheme: 'dark' });
    await applied('dark');
    await page.keyboard.press('Escape');
  },
  { engine: 'webkit' }
);
