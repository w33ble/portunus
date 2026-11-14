/** Languages the preview highlights; each one is a highlight.js grammar loaded on demand. */
export const LANGUAGES = [
  'bash',
  'c',
  'cpp',
  'css',
  'diff',
  'dockerfile',
  'go',
  'ini',
  'java',
  'javascript',
  'json',
  'makefile',
  'markdown',
  'nginx',
  'php',
  'python',
  'ruby',
  'rust',
  'sql',
  'typescript',
  'xml',
  'yaml',
] as const;

export type Language = (typeof LANGUAGES)[number];

/** Above this many bytes of text the preview stays plain: highlighting runs on the UI thread. */
export const HIGHLIGHT_LIMIT_BYTES = 200 * 1024;

function table(entries: Partial<Record<Language, string>>): Map<string, Language> {
  const map = new Map<string, Language>();
  for (const [language, keys] of Object.entries(entries)) {
    for (const key of keys.split(' ')) map.set(key, language as Language);
  }
  return map;
}

/** Whole file names, lower case. */
const BY_NAME = table({
  bash: '.bashrc .bash_profile .bash_login .bash_logout .bash_aliases .profile .zshrc .zshenv .zprofile .zlogin .zlogout .envrc .env pkgbuild',
  dockerfile: 'dockerfile containerfile',
  ini: '.gitconfig .gitmodules .editorconfig .npmrc .pylintrc .flake8',
  json: '.babelrc .eslintrc .prettierrc composer.lock',
  makefile: 'makefile gnumakefile',
  markdown: 'readme',
  nginx: 'nginx.conf',
  ruby: 'gemfile rakefile vagrantfile podfile guardfile brewfile',
});

/** Name prefixes before a variant suffix: `Dockerfile.dev`, `.env.local`. */
const BY_PREFIX: readonly (readonly [string, Language])[] = [
  ['dockerfile.', 'dockerfile'],
  ['containerfile.', 'dockerfile'],
  ['.env.', 'bash'],
];

/** Extensions without the dot, lower case. */
const BY_EXTENSION = table({
  bash: 'sh bash zsh ksh command',
  c: 'c h',
  cpp: 'cc cpp cxx c++ hh hpp hxx h++ ino',
  css: 'css',
  diff: 'diff patch',
  dockerfile: 'dockerfile',
  go: 'go',
  ini: 'ini cfg toml properties desktop service',
  java: 'java',
  javascript: 'js jsx mjs cjs',
  json: 'json jsonc geojson webmanifest',
  makefile: 'mk mak',
  markdown: 'md markdown mdown',
  nginx: 'nginx',
  php: 'php phtml',
  python: 'py pyw pyi',
  ruby: 'rb rake gemspec',
  rust: 'rs',
  sql: 'sql',
  typescript: 'ts tsx mts cts',
  xml: 'xml html htm xhtml svg plist xsd xsl xslt rss atom csproj',
  yaml: 'yaml yml',
});

/** Interpreter names from a shebang, with any trailing version (`python3.12`) removed. */
const BY_INTERPRETER = table({
  bash: 'sh bash zsh ksh dash ash',
  javascript: 'node nodejs deno bun',
  php: 'php',
  python: 'python pypy',
  ruby: 'ruby',
  typescript: 'ts-node tsx',
});

function fromShebang(firstLine: string): Language | null {
  if (!firstLine.startsWith('#!')) return null;
  const words = firstLine.slice(2).trim().split(/\s+/);
  let program = words.shift() ?? '';
  if (program === 'env' || program.endsWith('/env')) {
    // `env -S python3 -u` and `env FOO=1 python3`: the program is the first plain word.
    program = words.find((w) => !w.startsWith('-') && !w.includes('=')) ?? '';
  }
  const interpreter = (program.split('/').pop() ?? '').replace(/[\d.]+$/, '');
  return BY_INTERPRETER.get(interpreter) ?? null;
}

/**
 * The language to highlight `name` with, or null for plain text. Never guesses from content:
 * a known file name wins, then the extension, then a shebang on `firstLine`.
 */
export function languageFor(name: string, firstLine: string): Language | null {
  const lower = name.toLowerCase();
  const byName = BY_NAME.get(lower);
  if (byName !== undefined) return byName;
  for (const [prefix, language] of BY_PREFIX) {
    if (lower.startsWith(prefix)) return language;
  }
  const dot = lower.lastIndexOf('.');
  if (dot > 0) {
    const byExtension = BY_EXTENSION.get(lower.slice(dot + 1));
    if (byExtension !== undefined) return byExtension;
  }
  return fromShebang(firstLine);
}

/** The language for previewing `text` (`bytes` long) from file `name`, or null to stay plain. */
export function highlightLanguage(name: string, text: string, bytes: number): Language | null {
  if (bytes > HIGHLIGHT_LIMIT_BYTES) return null;
  const end = text.indexOf('\n');
  return languageFor(name, (end === -1 ? text : text.slice(0, end)).trimEnd());
}
