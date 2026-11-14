import { flow, assert, HOME } from './lib.mjs';

const DOCS = `${HOME}/Documents`;

/** `n` wheel events of `deltaX` over the pane's list, as a two-finger swipe sends them. */
async function swipe(page, pane, deltaX, n = 20) {
  const box = await pane.locator('[data-filelist]').boundingBox();
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  for (let i = 0; i < n; i++) await page.mouse.wheel(deltaX, 0);
}

/** Lets a gesture's momentum window close so the next swipe starts fresh. */
const settle = (page) => page.waitForTimeout(400);

const at = (pane, path) => pane.locator(`[data-filelist][aria-label="Files in ${path}"]`).waitFor();

async function intoPhotos({ page, left, row, goTo }) {
  await goTo(left, DOCS);
  // The double-click's first click selects photos: that's what back reselects.
  await row(left, 'photos').dblclick();
  await at(left, `${DOCS}/photos`);
  await row(left, '2026').waitFor();
  await settle(page);
}

async function swipesAndKeys(ctx) {
  const { page, left, row, step, shot } = ctx;
  await intoPhotos(ctx);

  step('a short swipe shows the arrow without navigating');
  await swipe(page, left, -20, 5);
  // Keep the gesture alive (100 of 150 px so far) while the arrow is checked and captured.
  const holding = (async () => {
    for (let i = 0; i < 10; i++) {
      await page.mouse.wheel(-1, 0);
      await page.waitForTimeout(40);
    }
  })();
  await left.locator('[aria-hidden="true"][data-direction="back"]').waitFor({ timeout: 1000 });
  await shot('swipe-arrow', { settle: false });
  await holding;
  await settle(page);
  assert(
    (await left.locator('[data-direction]').count()) === 0,
    'arrow hidden once the gesture ends'
  );
  await at(left, `${DOCS}/photos`);

  step('a full swipe goes back and reselects what was selected there');
  await swipe(page, left, -20);
  await at(left, DOCS);
  await row(left, 'photos').and(page.locator('[aria-selected="true"]')).waitFor();
  await settle(page);

  step('a swipe the other way goes forward');
  await swipe(page, left, 20);
  await at(left, `${DOCS}/photos`);
  await settle(page);

  step('keyboard back');
  await left.locator('[data-filelist]').focus();
  await page.keyboard.press('Alt+ArrowLeft');
  await at(left, DOCS);
}

await flow('history', async (ctx) => {
  const { page, left, right, step } = ctx;
  await swipesAndKeys(ctx);

  // Playwright's mouse API has no back/forward buttons; CDP does (Chromium only).
  const cdp = await page.context().newCDPSession(page);
  const click = async (pane, button) => {
    const box = await pane.locator('[data-filelist]').boundingBox();
    const at = { x: box.x + box.width / 2, y: box.y + box.height / 2, button, clickCount: 1 };
    await cdp.send('Input.dispatchMouseEvent', { type: 'mousePressed', ...at });
    await cdp.send('Input.dispatchMouseEvent', { type: 'mouseReleased', ...at });
  };

  step('mouse forward and back buttons on the pane under the pointer');
  await click(left, 'forward');
  await at(left, `${DOCS}/photos`);
  await click(right, 'back');
  await page.waitForTimeout(300);
  await at(left, `${DOCS}/photos`);
  await click(left, 'back');
  await at(left, DOCS);
  assert(page.url() === process.env.APP_URL, `page navigated to ${page.url()}`);
});

// WebKit: the closest stand-in for macOS WKWebView's wheel events.
await flow('history-webkit', swipesAndKeys, { engine: 'webkit' });
