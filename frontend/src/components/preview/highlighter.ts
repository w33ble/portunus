import type { HLJSApi, LanguageFn } from 'highlight.js';
import type { Language } from '../../lib/syntax';

/** One dynamic import per grammar, so Vite emits one lazy chunk each and nothing loads until used. */
const GRAMMARS: Record<Language, () => Promise<{ default: LanguageFn }>> = {
  bash: () => import('highlight.js/lib/languages/bash'),
  c: () => import('highlight.js/lib/languages/c'),
  cpp: () => import('highlight.js/lib/languages/cpp'),
  css: () => import('highlight.js/lib/languages/css'),
  diff: () => import('highlight.js/lib/languages/diff'),
  dockerfile: () => import('highlight.js/lib/languages/dockerfile'),
  go: () => import('highlight.js/lib/languages/go'),
  ini: () => import('highlight.js/lib/languages/ini'),
  java: () => import('highlight.js/lib/languages/java'),
  javascript: () => import('highlight.js/lib/languages/javascript'),
  json: () => import('highlight.js/lib/languages/json'),
  makefile: () => import('highlight.js/lib/languages/makefile'),
  markdown: () => import('highlight.js/lib/languages/markdown'),
  nginx: () => import('highlight.js/lib/languages/nginx'),
  php: () => import('highlight.js/lib/languages/php'),
  python: () => import('highlight.js/lib/languages/python'),
  ruby: () => import('highlight.js/lib/languages/ruby'),
  rust: () => import('highlight.js/lib/languages/rust'),
  sql: () => import('highlight.js/lib/languages/sql'),
  typescript: () => import('highlight.js/lib/languages/typescript'),
  xml: () => import('highlight.js/lib/languages/xml'),
  yaml: () => import('highlight.js/lib/languages/yaml'),
};

let core: Promise<HLJSApi> | null = null;
const registered = new Map<Language, Promise<void>>();

function loadCore(): Promise<HLJSApi> {
  core ??= import('highlight.js/lib/core').then((m) => m.default);
  return core;
}

/**
 * highlight.js markup for `text` as `language`. The result is highlight.js output only, which
 * escapes its input, so it is safe to set as innerHTML. Loads the core and the grammar on first use.
 */
export async function highlight(language: Language, text: string): Promise<string> {
  const hljs = await loadCore();
  let ready = registered.get(language);
  if (ready === undefined) {
    ready = GRAMMARS[language]().then((m) => hljs.registerLanguage(language, m.default));
    registered.set(language, ready);
  }
  await ready;
  return hljs.highlight(text, { language, ignoreIllegals: true }).value;
}
