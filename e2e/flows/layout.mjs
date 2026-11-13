import { flow, assert } from './lib.mjs';

// Window widths from the browser-only build's narrowest useful size up to a typical desktop.
const WIDTHS = [600, 700, 760, 900, 1100, 1440];

await flow('layout', async ({ page, left, row, step, shot }) => {
  await row(left, 'Documents').waitFor();

  for (const width of WIDTHS) {
    step(`panes stay inside the divider at ${width}px`);
    await page.setViewportSize({ width, height: 700 });

    const panes = await page.evaluate(() =>
      [...document.querySelectorAll('section[data-pane]')].map((pane) => {
        const visible = (label) => {
          const cell = [...pane.querySelectorAll('[role="columnheader"]')].find(
            (c) => c.textContent.trim().startsWith(label)
          );
          return cell ? cell.getBoundingClientRect().width : 0;
        };
        return {
          id: pane.dataset.pane,
          scrollWidth: pane.scrollWidth,
          clientWidth: pane.clientWidth,
          right: pane.getBoundingClientRect().right,
          headerRight: pane.firstElementChild.getBoundingClientRect().right,
          name: visible('Name'),
          size: visible('Size'),
          modified: visible('Modified'),
        };
      })
    );

    for (const p of panes) {
      const at = `${p.id} pane at ${width}px`;
      assert(p.scrollWidth <= p.clientWidth, `${at}: scrollWidth ${p.scrollWidth} > ${p.clientWidth}`);
      assert(p.headerRight <= p.right + 0.5, `${at}: header ends at ${p.headerRight}, pane at ${p.right}`);
      assert(p.name >= 200, `${at}: Name column only ${p.name}px`);
      assert(p.size > 0 || p.modified === 0, `${at}: Modified shown while Size is hidden`);
    }
  }

  await page.setViewportSize({ width: 700, height: 700 });
  await shot('narrow');
});
