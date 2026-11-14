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

    step('a Dockerfile is highlighted by name, a diff by extension');
    panel = await openPreview(ctx, left, 'Dockerfile');
    await panel.locator('pre .hljs-keyword', { hasText: 'FROM' }).first().waitFor({ timeout: 15000 });
    panel = await openPreview(ctx, left, 'sample.patch');
    await panel.locator('pre .hljs-addition').first().waitFor({ timeout: 15000 });
    await panel.locator('pre .hljs-deletion').first().waitFor();
    await shot('diff');

    step('Wrap still toggles on highlighted code');
    panel = await openPreview(ctx, left, 'sample.go');
    await panel.locator('pre .hljs-keyword').first().waitFor({ timeout: 15000 });
    const pre = panel.locator('pre');
    const whiteSpace = () => pre.evaluate((el) => getComputedStyle(el).whiteSpace);
    assert((await whiteSpace()) === 'pre-wrap', 'wrapped by default');
    await panel.getByRole('button', { name: 'Wrap' }).click();
    assert((await whiteSpace()) === 'pre', 'Wrap off');
    await panel.getByRole('button', { name: 'Wrap' }).click();

    step('moving off code before it is highlighted never leaves its markup on the next file');
    await row(left, 'sample.go').click();
    await row(left, 'hello.txt').click();
    await panel.locator('h2', { hasText: 'hello.txt' }).waitFor();
    await panel.locator('pre', { hasText: 'hello from portunus' }).waitFor({ timeout: 15000 });
    await page.waitForTimeout(500);
    assert((await panel.locator('pre [class*="hljs"]').count()) === 0, 'plain file has no markup');

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
