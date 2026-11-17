// Captures the README screenshots from the demo build (frontend/demo-dist).
// Runs in the Playwright image with the repo mounted at /work; see the docs:screenshots mise task.
import { createReadStream } from 'node:fs';
import { mkdir, readFile, stat, writeFile } from 'node:fs/promises';
import { createServer } from 'node:http';
import { createRequire } from 'node:module';
import { extname, join, normalize } from 'node:path';

const { chromium } = createRequire('/work/e2e/package.json')('playwright');

const SITE = '/work/frontend/demo-dist';
const OUT = '/work/docs/screenshots';
const THUMBS = join(OUT, 'thumbs');
const THUMB_WIDTH = 640;
// The fixtures' dates are relative to now, so a fixed clock keeps every capture identical.
const NOW = new Date('2026-06-18T16:20:00Z');

const TYPES = {
  '.html': 'text/html',
  '.js': 'text/javascript',
  '.css': 'text/css',
  '.png': 'image/png',
  '.ico': 'image/x-icon',
};

/** Serves the built demo on a free port of the container's loopback. */
async function serve() {
  const server = createServer(async (req, res) => {
    const url = new URL(req.url ?? '/', 'http://localhost');
    const path = normalize(join(SITE, url.pathname === '/' ? 'index.html' : url.pathname));
    const ok =
      path.startsWith(SITE) &&
      (await stat(path).then(
        (s) => s.isFile(),
        () => false
      ));
    res.writeHead(ok ? 200 : 404, {
      'Content-Type': TYPES[extname(path)] ?? 'application/octet-stream',
    });
    if (ok) createReadStream(path).pipe(res);
    else res.end();
  });
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  return { server, url: `http://127.0.0.1:${server.address().port}/` };
}

/** Writes docs/screenshots/<name>.png once the UI has stopped moving. */
async function shot(page, name) {
  await page.mouse.move(1100, 790); // off every row, so no hover highlight
  await page.waitForTimeout(600);
  await page.screenshot({ path: join(OUT, `${name}.png`), animations: 'disabled' });
  console.log(`captured ${name}`);
}

const row = (pane, text) => pane.locator('[role="option"]', { hasText: text }).first();

async function capture(browser, url) {
  const context = await browser.newContext({
    viewport: { width: 1280, height: 800 },
    deviceScaleFactor: 2,
    locale: 'en-US',
    timezoneId: 'America/New_York',
    colorScheme: 'light',
  });
  const page = await context.newPage();
  const problems = [];
  page.on('pageerror', (e) => problems.push(e.message));
  page.on('console', (m) => m.type() === 'error' && problems.push(m.text()));
  await page.clock.setFixedTime(NOW);
  await page.goto(url);

  const left = page.locator('[data-pane="left"]');
  const right = page.locator('[data-pane="right"]');
  const dialog = page.locator('dialog[open]');
  const openPicker = async (pane) => {
    await pane.getByRole('button', { name: 'Connection', exact: true }).click();
    await page.getByRole('listbox', { name: 'Connect to' }).waitFor();
  };

  await row(left, 'Projects').waitFor();
  await left.getByLabel('Edit path').click();
  await left.locator('input[aria-label="Path"]').fill('/Users/joe/Projects/website');
  await left.locator('input[aria-label="Path"]').press('Enter');
  await row(left, 'deploy.sh').waitFor();
  await row(left, 'dist').click();
  await row(left, 'deploy.sh').click({ modifiers: ['Meta'] });

  await openPicker(right);
  await page
    .getByRole('listbox', { name: 'Connect to' })
    .getByRole('option', { name: 'web-prod' })
    .click();
  await row(right, 'robots.txt').waitFor();
  // The left pane's picker, so the remote listing on the right stays readable.
  await openPicker(left);
  await shot(page, 'browser');
  await page.keyboard.press('Escape');

  await page.getByRole('tab', { name: /^Transfers/ }).click();
  await page.getByText('dist/ and 1 more').waitFor();
  await shot(page, 'transfers');

  await page.getByRole('tab', { name: 'Browser' }).click();
  await openPicker(right);
  await page.getByRole('button', { name: 'Manage connections…', exact: true }).click();
  await dialog.getByText('deploy@web1.example.com').waitFor();
  await shot(page, 'connections');
  await dialog.getByRole('button', { name: 'Done', exact: true }).click();
  await dialog.waitFor({ state: 'detached' });

  await page.getByLabel('Settings').click();
  await dialog.getByRole('tab', { name: 'Appearance' }).click();
  await dialog.getByText('Tokyo Night', { exact: true }).click();
  await page.waitForFunction(() => document.documentElement.dataset.theme === 'tokyo-night');
  await shot(page, 'themes');

  await context.close();
  if (problems.length > 0) throw new Error(`the page reported errors:\n${problems.join('\n')}`);
}

/** Renders each screenshot as a 640px-wide <img> and saves that element at device scale 1. */
async function thumbnails(browser, names) {
  const context = await browser.newContext({
    viewport: { width: THUMB_WIDTH, height: 400 },
    deviceScaleFactor: 1,
  });
  const page = await context.newPage();
  for (const name of names) {
    const png = await readFile(join(OUT, `${name}.png`));
    await page.setContent(
      `<body style="margin:0"><img style="display:block;width:${THUMB_WIDTH}px" src="data:image/png;base64,${png.toString('base64')}"></body>`
    );
    const img = page.locator('img');
    await img.evaluate((el) => el.decode());
    await writeFile(join(THUMBS, `${name}.png`), await img.screenshot());
    console.log(`thumbnail ${name}`);
  }
  await context.close();
}

const NAMES = ['browser', 'transfers', 'connections', 'themes'];

await mkdir(THUMBS, { recursive: true });
const { server, url } = await serve();
const browser = await chromium.launch();
try {
  await capture(browser, url);
  await thumbnails(browser, NAMES);
} finally {
  await browser.close();
  server.close();
}
