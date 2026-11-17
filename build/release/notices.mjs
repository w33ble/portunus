// Builds THIRD_PARTY_NOTICES.md from go-licenses' saved license files, pnpm's license list and
// the licenses of the color themes' palettes (palettes/<name>/LICENSE, next to this script).
// Identical license texts are printed once with every package that ships them.
// Usage: node notices.mjs <pnpm licenses list --json file> <go-licenses save dir>...
import { createHash } from 'node:crypto';
import { existsSync, readFileSync, readdirSync, statSync } from 'node:fs';
import { join, relative } from 'node:path';

const [npmJson, ...goDirs] = process.argv.slice(2);
if (!npmJson || goDirs.length === 0) {
  console.error('usage: node notices.mjs <pnpm-licenses.json> <go-licenses-dir>...');
  process.exit(2);
}

const LICENSE_FILE = /^(licen[cs]e|copying|notice)([.-].*)?$/i;

/** text hash -> { text, packages: Set<string> } */
const groups = new Map();

function add(pkg, text) {
  const normalized = text.replace(/\r\n/g, '\n').trim();
  const key = createHash('sha256').update(normalized).digest('hex');
  const group = groups.get(key) ?? { text: normalized, packages: new Set() };
  group.packages.add(pkg);
  groups.set(key, group);
}

function walk(root, dir, kind) {
  for (const name of readdirSync(dir)) {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) walk(root, path, kind);
    else if (LICENSE_FILE.test(name)) add(`${relative(root, dir)} (${kind})`, readFileSync(path, 'utf8'));
  }
}
for (const root of goDirs) walk(root, root, 'Go');
const palettes = join(import.meta.dirname, 'palettes');
walk(palettes, palettes, 'color palette');

const npm = JSON.parse(readFileSync(npmJson, 'utf8'));
for (const entries of Object.values(npm)) {
  for (const entry of entries) {
    for (const [i, path] of entry.paths.entries()) {
      const label = `${entry.name}@${entry.versions[i] ?? entry.versions[0]} (npm)`;
      const files = existsSync(path) ? readdirSync(path).filter((f) => LICENSE_FILE.test(f)) : [];
      if (files.length === 0) {
        add(label, `${entry.license} (no license file shipped; see ${entry.homepage ?? 'package registry'})`);
      }
      for (const file of files) add(label, readFileSync(join(path, file), 'utf8'));
    }
  }
}

const sorted = [...groups.values()]
  .map((g) => ({ ...g, packages: [...g.packages].sort() }))
  .sort((a, b) => a.packages[0].localeCompare(b.packages[0]));

const out = [
  '# Third-party notices',
  '',
  'Portunus is licensed under the Mozilla Public License 2.0. It includes the following third-party',
  'software, distributed under their own licenses.',
  '',
];
for (const g of sorted) {
  out.push(`## ${g.packages.join(', ')}`, '', '```', g.text, '```', '');
}
process.stdout.write(out.join('\n'));
