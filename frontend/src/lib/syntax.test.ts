import { describe, expect, it } from 'vitest';
import { HIGHLIGHT_LIMIT_BYTES, highlightLanguage, languageFor } from './syntax';

describe('languageFor', () => {
  it.each([
    ['main.go', 'go'],
    ['app.ts', 'typescript'],
    ['App.TSX', 'typescript'],
    ['index.mjs', 'javascript'],
    ['tool.py', 'python'],
    ['data.json', 'json'],
    ['ci.yml', 'yaml'],
    ['Cargo.toml', 'ini'],
    ['page.html', 'xml'],
    ['logo.svg', 'xml'],
    ['style.css', 'css'],
    ['README.md', 'markdown'],
    ['run.sh', 'bash'],
    ['lib.rs', 'rust'],
    ['util.h', 'c'],
    ['util.hpp', 'cpp'],
    ['Main.java', 'java'],
    ['task.rb', 'ruby'],
    ['index.php', 'php'],
    ['schema.sql', 'sql'],
    ['fix.patch', 'diff'],
    ['rules.mk', 'makefile'],
    ['archive.tar.gz.py', 'python'],
  ])('picks %s by extension: %s', (name, language) => {
    expect(languageFor(name, '')).toBe(language);
  });

  it.each([
    ['Dockerfile', 'dockerfile'],
    ['dockerfile', 'dockerfile'],
    ['Dockerfile.dev', 'dockerfile'],
    ['Containerfile', 'dockerfile'],
    ['Makefile', 'makefile'],
    ['GNUmakefile', 'makefile'],
    ['.bashrc', 'bash'],
    ['.zshrc', 'bash'],
    ['.env', 'bash'],
    ['.env.local', 'bash'],
    ['.gitconfig', 'ini'],
    ['Gemfile', 'ruby'],
    ['nginx.conf', 'nginx'],
  ])('picks %s by file name: %s', (name, language) => {
    expect(languageFor(name, '')).toBe(language);
  });

  it('prefers a known name over its extension', () => {
    expect(languageFor('Dockerfile.sh', '')).toBe('dockerfile');
  });

  it.each([
    ['#!/bin/bash', 'bash'],
    ['#!/bin/sh', 'bash'],
    ['#!/usr/bin/env zsh', 'bash'],
    ['#!/usr/bin/env python3', 'python'],
    ['#!/usr/bin/python3.12 -u', 'python'],
    ['#!/usr/bin/env -S node --no-warnings', 'javascript'],
    ['#!/usr/bin/env FOO=1 ruby', 'ruby'],
    ['#!/usr/bin/php', 'php'],
  ])('picks a script by its shebang %s: %s', (firstLine, language) => {
    expect(languageFor('deploy', firstLine)).toBe(language);
  });

  it('uses the shebang for an unrecognised extension but not over a known one', () => {
    expect(languageFor('build.run', '#!/bin/bash')).toBe('bash');
    expect(languageFor('tool.go', '#!/bin/bash')).toBe('go');
  });

  it.each([
    ['notes.txt', ''],
    ['archive', ''],
    ['.hidden', ''],
    ['photo.jpeg', ''],
    ['script', '#!/usr/bin/perl'],
    ['script', '# not a shebang'],
    ['script', '#!'],
    ['script', '#!/usr/bin/env'],
  ])('leaves %s with first line %j as plain text', (name, firstLine) => {
    expect(languageFor(name, firstLine)).toBeNull();
  });
});

describe('highlightLanguage', () => {
  it('reads the shebang from the first line of the text, tolerating CRLF', () => {
    expect(highlightLanguage('deploy', '#!/bin/bash\r\necho hi\n', 20)).toBe('bash');
    expect(highlightLanguage('deploy', 'echo hi\n#!/bin/bash\n', 20)).toBeNull();
  });

  it('highlights text up to the size cap and leaves larger text plain', () => {
    expect(highlightLanguage('main.go', 'package main', HIGHLIGHT_LIMIT_BYTES)).toBe('go');
    expect(highlightLanguage('main.go', 'package main', HIGHLIGHT_LIMIT_BYTES + 1)).toBeNull();
  });
});
