// All vfs paths are absolute and '/'-separated on every backend ("C:/Users/x" on Windows).

const DRIVE = /^[A-Za-z]:$/;

export interface Crumb {
  label: string;
  path: string;
}

/** Collapses repeated slashes and drops a trailing slash, keeping roots ("/", "C:/") intact. */
export function normalizePath(input: string): string {
  const trimmed = input
    .trim()
    .replaceAll('\\', '/')
    .replace(/\/{2,}/g, '/');
  if (trimmed === '') return '/';
  const body = trimmed.length > 1 ? trimmed.replace(/\/$/, '') : trimmed;
  return DRIVE.test(body) ? `${body}/` : body;
}

function isRoot(p: string): boolean {
  return p === '/' || /^[A-Za-z]:\/$/.test(p);
}

export function parentPath(p: string): string {
  const norm = normalizePath(p);
  if (isRoot(norm)) return norm;
  const cut = norm.lastIndexOf('/');
  if (cut <= 0) return '/';
  const parent = norm.slice(0, cut);
  return DRIVE.test(parent) ? `${parent}/` : parent;
}

export function joinPath(dir: string, name: string): string {
  return dir.endsWith('/') ? `${dir}${name}` : `${dir}/${name}`;
}

export function baseName(p: string): string {
  const norm = normalizePath(p);
  if (isRoot(norm)) return norm;
  return norm.slice(norm.lastIndexOf('/') + 1);
}

export function breadcrumbs(p: string): Crumb[] {
  const norm = normalizePath(p);
  const drive = /^([A-Za-z]:)\//.exec(norm);
  const rootPath = drive ? `${drive[1]}/` : '/';
  const crumbs: Crumb[] = [{ label: drive?.[1] ?? '/', path: rootPath }];

  let acc = rootPath;
  for (const seg of norm.slice(rootPath.length).split('/')) {
    if (seg === '') continue;
    acc = joinPath(acc, seg);
    crumbs.push({ label: seg, path: acc });
  }
  return crumbs;
}
