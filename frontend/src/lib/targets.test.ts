import { describe, expect, it } from 'vitest';
import { TargetKind } from '../api/enums';
import { quickConnectLabel, targetKey } from './targets';

describe('targetKey', () => {
  it('distinguishes aliases, quick connects and profiles', () => {
    expect(targetKey({ kind: TargetKind.TargetLocal })).toBe('local');
    expect(targetKey({ kind: TargetKind.TargetSSH, alias: 'prod' })).toBe('ssh:prod');
    expect(targetKey({ kind: TargetKind.TargetSSH, user: 'u', host: 'h', port: 22 })).toBe(
      'quick:u@h:22'
    );
    expect(targetKey({ kind: TargetKind.TargetS3, profileId: 'p1' })).toBe('s3:p1');
  });
});

describe('quickConnectLabel', () => {
  it('re-brackets IPv6 and omits empty parts', () => {
    expect(quickConnectLabel('u', 'h', 22)).toBe('u@h:22');
    expect(quickConnectLabel('', '::1', 0)).toBe('[::1]');
  });
});
