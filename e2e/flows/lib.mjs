import { chromium, webkit } from 'playwright';
import { readFileSync, existsSync, readdirSync } from 'node:fs';

export const E2E_DIR = process.env.E2E_DIR;
export const HOME = `${E2E_DIR}/home`;

/** Runs a named flow with a fresh page, screenshots on failure, exit code 1 on error. */
export async function flow(name, fn, { engine = 'chromium' } = {}) {
  const browser = await (engine === 'webkit' ? webkit : chromium).launch();
  // axe-core needs a page from an explicit context, not browser.newPage().
  const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  const page = await context.newPage();
  const errors = [];
  page.on('pageerror', (e) => errors.push(e.message));

  let shotN = 0;
  const ctx = {
    page,
    left: page.locator('[data-pane="left"]'),
    right: page.locator('[data-pane="right"]'),
    row: (pane, text) => pane.locator('[role="option"]', { hasText: text }).first(),
    step: (s) => console.log(`  ${name}: ${s}`),
    /** `settle: false` captures at once, for state that lasts less than the usual 400 ms wait. */
    shot: async (label, { settle = true } = {}) => {
      if (settle) await page.waitForTimeout(400); // let dialog transitions settle
      await page.screenshot({ path: `/out/${name}-${String(++shotN).padStart(2, '0')}-${label}.png` });
    },
    goTo: async (pane, p) => {
      await pane.getByLabel('Edit path').click();
      const input = pane.locator('input[aria-label="Path"]');
      await input.fill(p);
      await input.press('Enter');
    },
  };

  try {
    await page.goto(process.env.APP_URL);
    await fn(ctx);
    if (errors.length) throw new Error(`page errors:\n${errors.join('\n')}`);
    console.log(`PASS ${name}`);
  } catch (err) {
    await ctx.shot('failure').catch(() => {});
    console.error(`FAIL ${name}: ${err.message}`);
    process.exitCode = 1;
  } finally {
    await browser.close();
  }
}

export function assert(cond, msg) {
  if (!cond) throw new Error(`assertion failed: ${msg}`);
}

export function sameFile(a, b) {
  return existsSync(a) && existsSync(b) && readFileSync(a).equals(readFileSync(b));
}

export function count(dir) {
  return existsSync(dir) ? readdirSync(dir).length : -1;
}

/** Opens the quick preview on `name` in `pane` (or moves it there) and returns the panel. */
export async function openPreview({ page, row }, pane, name) {
  const panel = page.locator('section[data-preview]');
  await row(pane, name).click();
  if (!(await panel.isVisible())) await page.keyboard.press('Space');
  await panel.locator('h2', { hasText: name }).waitFor({ timeout: 15000 });
  return panel;
}

/** Previews the hello.txt, sample.go, pixel.png and doc.pdf fixtures in `pane`, then closes the preview. */
export async function previewFixtures(ctx, pane, where) {
  const { page, step, shot } = ctx;

  step(`preview text on ${where}`);
  let panel = await openPreview(ctx, pane, 'hello.txt');
  await panel.locator('pre', { hasText: 'hello from portunus' }).waitFor({ timeout: 15000 });
  assert((await panel.locator('pre [class*="hljs"]').count()) === 0, 'plain text stays unhighlighted');

  step(`preview highlighted code on ${where}`);
  panel = await openPreview(ctx, pane, 'sample.go');
  await panel.locator('pre .hljs-keyword', { hasText: 'package' }).first().waitFor({ timeout: 15000 });
  await panel.locator('pre .hljs-string').first().waitFor();
  await shot(`preview-code-${where}`);

  step(`preview image on ${where}`);
  panel = await openPreview(ctx, pane, 'pixel.png');
  const width = await panel.locator('img').evaluate(async (img) => {
    await img.decode();
    return img.naturalWidth;
  });
  assert(width === 16, `pixel.png decoded ${width}px wide, want 16`);
  await shot(`preview-image-${where}`);

  step(`preview PDF on ${where}`);
  const viewer = await page.evaluate(() => navigator.pdfViewerEnabled === true);
  // The viewer's own GET; whether it renders is the browser's business (Playwright's WebKit
  // build claims a viewer but leaves the frame blank).
  const served = viewer
    ? page.waitForResponse(
        (r) => r.url().includes('/preview') && r.request().method() === 'GET',
        { timeout: 15000 }
      )
    : null;
  panel = await openPreview(ctx, pane, 'doc.pdf');
  if (served) {
    await panel.locator('iframe').waitFor({ timeout: 15000 });
    const res = await served;
    assert(
      res.status() === 200 && res.headers()['content-type'] === 'application/pdf',
      `PDF served as ${res.status()} ${res.headers()['content-type']}`
    );
  } else {
    await panel.getByText('No preview available').waitFor({ timeout: 15000 });
  }
  step(`  PDF viewer ${viewer ? 'loaded' : 'unavailable in this browser, metadata only'}`);
  await shot(`preview-pdf-${where}`);

  await page.keyboard.press('Escape');
  await panel.waitFor({ state: 'detached' });
}
