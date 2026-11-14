import { flow, assert, openPreview, previewFixtures, HOME } from './lib.mjs';

// WebKit: the closest stand-in for macOS WKWebView.
await flow(
  'preview',
  async (ctx) => {
    const { page, left, row, step, shot, goTo } = ctx;
    const right = page.locator('section[data-pane="right"]');
    await goTo(left, `${HOME}/Documents`);
    await row(left, 'hello.txt').waitFor();

    await previewFixtures(ctx, left, 'local');

    step('preview stands in for the other pane and follows the cursor');
    let panel = await openPreview(ctx, left, 'hello.txt');
    assert(await right.isHidden(), 'right pane hidden while previewing');
    await page.keyboard.press('ArrowDown');
    await panel.locator('h2', { hasText: 'pixel.png' }).waitFor();
    await panel.locator('img').waitFor();

    step('binary file shows metadata only');
    panel = await openPreview(ctx, left, 'random.bin');
    await panel.getByText('No preview available').waitFor({ timeout: 15000 });
    await panel.locator('dl', { hasText: '3.0 MB' }).waitFor();
    await shot('binary');

    step('folder shows a summary');
    panel = await openPreview(ctx, left, 'photos');
    await panel.getByText('2 items').waitFor({ timeout: 15000 });
    await panel.getByText('2 folders, 0 files').waitFor();
    await shot('folder');

    step('Space closes it and the other pane comes back');
    await page.keyboard.press('Space');
    await panel.waitFor({ state: 'detached' });
    assert(await right.isVisible(), 'right pane visible again');
  },
  { engine: 'webkit' }
);
