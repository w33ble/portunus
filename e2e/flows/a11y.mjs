import { AxeBuilder } from '@axe-core/playwright';
import { writeFileSync } from 'node:fs';

/** WCAG 2.x minimums. Disabled controls are exempt from AA but must stay legible, so they get the UI-component floor. */
export const MIN = { text: 4.5, placeholder: 4.5, icon: 3, disabled: 3 };

/** Runs in the page: every piece of text and every icon inside `scope`, with its colours composited over whatever is behind it. */
function measure(scope) {
  const channels = (s) => (s.match(/-?[\d.]+%?/g) ?? []).map((n) => (n.endsWith('%') ? parseFloat(n) / 100 : parseFloat(n)));
  const parse = (s) => {
    const n = channels(s);
    const scale = s.startsWith('color(') ? 255 : 1;
    return [n[0] * scale, n[1] * scale, n[2] * scale, n[3] ?? 1];
  };
  const over = (top, under) => {
    const a = top[3];
    return [0, 1, 2].map((i) => top[i] * a + under[i] * (1 - a)).concat(1);
  };
  const lum = (c) => {
    const [r, g, b] = c.slice(0, 3).map((v) => {
      const s = v / 255;
      return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
    });
    return 0.2126 * r + 0.7152 * g + 0.0722 * b;
  };
  const ratio = (a, b) => {
    const [hi, lo] = [lum(a), lum(b)].sort((x, y) => y - x);
    return (hi + 0.05) / (lo + 0.05);
  };
  const backdrop = (el) => {
    const layers = [];
    for (let e = el; e; e = e.parentElement) {
      const bg = parse(getComputedStyle(e).backgroundColor);
      if (bg[3] > 0) layers.push(bg);
      if (bg[3] === 1) break;
    }
    return layers.reduceRight((under, layer) => over(layer, under), [255, 255, 255, 1]);
  };
  const rows = [];
  const add = (kind, el, text, fgCss) => {
    const bg = backdrop(el);
    const fg = over(parse(fgCss), bg);
    const hex = (c) => '#' + c.slice(0, 3).map((v) => Math.round(v).toString(16).padStart(2, '0')).join('');
    rows.push({ kind, text: text.trim().slice(0, 48), fg: hex(fg), bg: hex(bg), ratio: Math.round(ratio(fg, bg) * 100) / 100 });
  };
  const visible = (el) => {
    const r = el.getBoundingClientRect();
    return r.width > 0 && r.height > 0 && getComputedStyle(el).visibility === 'visible';
  };

  const root = document.querySelector(scope);
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  for (let n = walker.nextNode(); n; n = walker.nextNode()) {
    const el = n.parentElement;
    if (!n.textContent.trim() || !visible(el)) continue;
    add(el.closest(':disabled') ? 'disabled' : 'text', el, n.textContent, getComputedStyle(el).color);
  }
  for (const input of root.querySelectorAll('input[placeholder]')) {
    if (input.value === '' && visible(input)) {
      add(input.disabled ? 'disabled' : 'placeholder', input, input.placeholder, getComputedStyle(input, '::placeholder').color);
    }
  }
  for (const svg of root.querySelectorAll('button:not(:disabled) svg')) {
    if (visible(svg)) add('icon', svg, svg.closest('button').getAttribute('aria-label') ?? 'icon', getComputedStyle(svg).color);
  }
  return rows;
}

/** Every theme the stylesheets define, read from their [data-theme] selectors. */
function themeIds() {
  const ids = new Set();
  const walk = (rules) => {
    for (const rule of rules) {
      if (rule.cssRules) walk(rule.cssRules);
      for (const m of (rule.selectorText ?? '').matchAll(/\[data-theme=["']?([\w-]+)/g)) ids.add(m[1]);
    }
  };
  for (const sheet of document.styleSheets) walk(sheet.cssRules);
  return [...ids];
}

/** Collects violations across a whole flow so one run reports every failing dialog. */
export function contrastAudit(page) {
  const failures = [];
  const report = [];
  let themes;

  return {
    /**
     * Checks the open dialog (or `scope`) in every theme. axe covers enabled text; the custom pass
     * covers what axe skips, and with `strictText` also enforces the text floor itself.
     */
    async check(label, { scope = 'dialog[open]', strictText = false } = {}) {
      themes ??= await page.evaluate(themeIds);
      if (themes.length < 2) throw new Error(`found themes ${themes}, want them all`);
      const saved = await page.evaluate(() => document.documentElement.dataset.theme);
      for (const theme of themes) {
        await page.evaluate((t) => (document.documentElement.dataset.theme = t), theme);
        await page.mouse.move(0, 0);
        await page.waitForTimeout(300); // button background transitions
        const where = `${label} [${theme}]`;
        const before = failures.length;

        const axe = await new AxeBuilder({ page }).include(scope).withRules(['color-contrast']).analyze();
        for (const v of axe.violations) {
          for (const node of v.nodes) {
            const d = node.any[0]?.data ?? {};
            failures.push(`${where}: axe ${node.target} ${d.fgColor} on ${d.bgColor} = ${d.contrastRatio}:1 (needs ${d.expectedContrastRatio})`);
          }
        }

        const rows = await page.evaluate(measure, scope);
        for (const r of rows) {
          report.push({ dialog: label, theme, ...r });
          if ((strictText || r.kind !== 'text') && r.ratio < MIN[r.kind]) {
            failures.push(`${where}: ${r.kind} “${r.text}” ${r.fg} on ${r.bg} = ${r.ratio}:1 (needs ${MIN[r.kind]})`);
          }
        }
        if (failures.length > before) {
          await page.screenshot({ path: `/out/contrast-${label.replace(/\W+/g, '-')}-${theme}.png` });
        }
      }
      await page.evaluate((t) => (document.documentElement.dataset.theme = t), saved);
    },

    /** Writes the full measurement table and throws if anything failed. */
    finish() {
      writeFileSync('/out/contrast.json', JSON.stringify(report, null, 1));
      if (failures.length) throw new Error(`${failures.length} contrast failure(s):\n${failures.join('\n')}`);
    },
  };
}
