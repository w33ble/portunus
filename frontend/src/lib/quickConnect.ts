export interface QuickConnectTarget {
  user: string;
  host: string;
  /** 0 = use the ssh default. */
  port: number;
}

export type QuickConnectResult =
  | { ok: true; target: QuickConnectTarget }
  | { ok: false; error: string };

/**
 * Parses `[ssh://][user@]host[:port]`. IPv6 literals go in brackets: `user@[::1]:2222`.
 */
export function parseQuickConnect(input: string): QuickConnectResult {
  let rest = input
    .trim()
    .replace(/^ssh:\/\//i, '')
    .replace(/\/+$/, '');
  if (rest === '') return { ok: false, error: 'Enter user@host or host:port.' };
  if (/\s/.test(rest)) return { ok: false, error: 'The address can’t contain spaces.' };

  let user = '';
  const at = rest.lastIndexOf('@');
  if (at >= 0) {
    user = rest.slice(0, at);
    rest = rest.slice(at + 1);
    if (user === '') return { ok: false, error: 'The user name before @ is empty.' };
  }

  let host = rest;
  let portText: string | null = null;
  if (rest.startsWith('[')) {
    const close = rest.indexOf(']');
    if (close < 0) return { ok: false, error: 'Missing ] after the IPv6 address.' };
    host = rest.slice(1, close);
    const after = rest.slice(close + 1);
    if (after !== '') {
      if (!after.startsWith(':')) return { ok: false, error: 'Unexpected text after ].' };
      portText = after.slice(1);
    }
  } else {
    const colon = rest.lastIndexOf(':');
    if (colon >= 0) {
      if (rest.indexOf(':') !== colon) {
        return { ok: false, error: 'Put IPv6 addresses in brackets, like [::1]:22.' };
      }
      host = rest.slice(0, colon);
      portText = rest.slice(colon + 1);
    }
  }

  if (host === '') return { ok: false, error: 'The host is empty.' };

  let port = 0;
  if (portText !== null) {
    port = /^\d+$/.test(portText) ? Number(portText) : Number.NaN;
    if (!Number.isInteger(port) || port < 1 || port > 65535) {
      return { ok: false, error: 'The port must be a number from 1 to 65535.' };
    }
  }

  return { ok: true, target: { user, host, port } };
}
