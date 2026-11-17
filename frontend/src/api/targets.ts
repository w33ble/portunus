import { TargetKind } from './enums';
import type { Target } from './types';

/** Connect targets, built with the generated TargetKind values. No runtime imports. */
export const targets = {
  local: (): Target => ({ kind: TargetKind.TargetLocal }),
  ssh: (alias: string): Target => ({ kind: TargetKind.TargetSSH, alias }),
  quick: (user: string, host: string, port: number): Target => ({
    kind: TargetKind.TargetSSH,
    host,
    ...(user ? { user } : {}),
    ...(port ? { port } : {}),
  }),
  s3: (profileId: string): Target => ({ kind: TargetKind.TargetS3, profileId }),
};
