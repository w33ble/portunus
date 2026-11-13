import { TargetKind } from '../api/enums';
import type { Target } from '../api/types';

/** Stable identity of a connect target, used as the picker's option value. */
export function targetKey(t: Target): string {
  switch (t.kind) {
    case TargetKind.TargetLocal:
      return 'local';
    case TargetKind.TargetS3:
      return `s3:${t.profileId ?? ''}`;
    case TargetKind.TargetSSH:
      return t.alias ? `ssh:${t.alias}` : `quick:${t.user ?? ''}@${t.host ?? ''}:${t.port ?? 0}`;
    case TargetKind.$zero:
      return 'none';
    default:
      return `other:${String(t.kind satisfies never)}`;
  }
}

export function quickConnectLabel(user: string, host: string, port: number): string {
  const hostPart = host.includes(':') ? `[${host}]` : host;
  return `${user ? `${user}@` : ''}${hostPart}${port ? `:${port}` : ''}`;
}
