import { describe, expect, it } from 'vitest';
import type { HostKeyRequest, Pending, SecretRequest } from '../api/types';
import { enqueue, hostKeyPrompt, mergePending, resolvePrompt, secretPrompt } from './prompts';

const hkRequest: HostKeyRequest = {
  host: 'h',
  address: 'h:22',
  keyType: 'ssh-ed25519',
  fingerprint: 'SHA256:x',
  saveTo: '/k',
};
const secRequest: SecretRequest = { title: 't', message: '', label: 'Password', echo: false };
const hk = (id: string) => hostKeyPrompt({ id, request: hkRequest });
const sec = (id: string) => secretPrompt({ id, request: secRequest });

describe('prompt queue', () => {
  it('queues in arrival order and ignores duplicates', () => {
    let q = enqueue([], hk('1'));
    q = enqueue(q, sec('2'));
    expect(enqueue(q, hk('1'))).toBe(q);
    expect(q.map((p) => p.id)).toEqual(['1', '2']);
  });

  it('resolving the head exposes the next prompt', () => {
    const q = resolvePrompt([hk('1'), sec('2')], '1');
    expect(q[0]?.id).toBe('2');
  });

  it('resolving an unknown id is a no-op', () => {
    const q = [hk('1')];
    expect(resolvePrompt(q, '9')).toBe(q);
  });

  it('merges Pending() with events already queued, ordered by id', () => {
    const pending: Pending = {
      hostKeys: [{ id: '1', request: hkRequest }],
      secrets: [{ id: '3', request: secRequest }],
      conflicts: null,
    };
    const q = mergePending([sec('2')], pending);
    expect(q.map((p) => p.id)).toEqual(['1', '2', '3']);
  });

  it('merging an already-known prompt keeps the queue identity', () => {
    const q = [hk('1')];
    const pending: Pending = {
      hostKeys: [{ id: '1', request: hkRequest }],
      secrets: null,
      conflicts: null,
    };
    expect(mergePending(q, pending)).toBe(q);
  });
});
