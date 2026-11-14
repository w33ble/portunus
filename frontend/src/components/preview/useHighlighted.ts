import { useEffect, useState } from 'react';
import { highlightLanguage } from '../../lib/syntax';
import { highlight } from './highlighter';

/**
 * highlight.js markup for a previewed text file, or null while it is pending, when the file has
 * no language, or when it is over the size cap (the caller shows plain text). The markup is
 * tied to the `text` it came from, so a different file never shows the previous file's colors.
 */
export function useHighlighted(name: string, text: string, bytes: number): string | null {
  const language = highlightLanguage(name, text, bytes);
  const [result, setResult] = useState<{ text: string; language: string; html: string } | null>(
    null
  );

  useEffect(() => {
    if (language === null) return;
    let stale = false;
    highlight(language, text).then(
      (html) => {
        if (!stale) setResult({ text, language, html });
      },
      // Plain text is the fallback if a chunk fails to load.
      () => {}
    );
    return () => {
      stale = true;
    };
  }, [language, text]);

  return language !== null && result?.text === text && result.language === language
    ? result.html
    : null;
}
