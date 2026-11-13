import { chromium } from 'playwright';
import { readFileSync, existsSync, readdirSync } from 'node:fs';

export const E2E_DIR = process.env.E2E_DIR;
export const HOME = `${E2E_DIR}/home`;

/** Runs a named flow with a fresh page, screenshots on failure, exit code 1 on error. */
export async function flow(name, fn) {
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: 1280, height: 800 } });
  const errors = [];
  page.on('pageerror', (e) => errors.push(e.message));

  let shotN = 0;
  const ctx = {
    page,
    left: page.locator('[data-pane="left"]'),
    right: page.locator('[data-pane="right"]'),
    row: (pane, text) => pane.locator('[role="option"]', { hasText: text }).first(),
    step: (s) => console.log(`  ${name}: ${s}`),
    shot: async (label) => {
      await page.waitForTimeout(400); // let dialog transitions settle
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
