import { beforeEach, describe, expect, it, vi } from 'vitest';
import * as api from '../api';
import type { HostKeyRequest, SecretRequest } from '../api/types';
import { hostKeyPrompt, secretPrompt } from '../lib/prompts';
import { usePrompts } from './prompts';

vi.mock('../api', async () => (await import('./testApi')).createApiMock());

const hkRequest: HostKeyRequest = {
  host: 'h',
  address: 'h:22',
  keyType: 'ssh-ed25519',
  fingerprint: 'SHA256:x',
  saveTo: '/k',
};
const secRequest: SecretRequest = { title: 't', message: '', label: 'Password', echo: false };

const ids = () => usePrompts.getState().queue.map((p) => p.id);

beforeEach(() => {
  vi.clearAllMocks();
  usePrompts.setState(usePrompts.getInitialState(), true);
});

describe('usePrompts', () => {
  it('initializes from Pending() without duplicating events already received', async () => {
    usePrompts.getState().enqueue(secretPrompt({ id: '2', request: secRequest }));
    vi.mocked(api.prompts.pending).mockResolvedValue({
      hostKeys: [{ id: '1', request: hkRequest }],
      secrets: [{ id: '2', request: secRequest }],
      conflicts: null,
    });
    await usePrompts.getState().init();
    expect(ids()).toEqual(['1', '2']);
  });

  it('shows one prompt at a time: the head, then the next', async () => {
    usePrompts.getState().enqueue(hostKeyPrompt({ id: '1', request: hkRequest }));
    usePrompts.getState().enqueue(secretPrompt({ id: '2', request: secRequest }));
    expect(usePrompts.getState().queue[0]?.id).toBe('1');

    vi.mocked(api.prompts.respondHostKey).mockResolvedValue();
    await usePrompts.getState().respondHostKey('1', true);
    expect(api.prompts.respondHostKey).toHaveBeenCalledWith('1', true);
    expect(usePrompts.getState().queue[0]?.id).toBe('2');
  });

  it('dismisses a prompt resolved elsewhere', () => {
    usePrompts.getState().enqueue(hostKeyPrompt({ id: '1', request: hkRequest }));
    usePrompts.getState().resolve('1');
    expect(ids()).toEqual([]);
  });

  it('sends secrets and cancellation through the same call', async () => {
    usePrompts.getState().enqueue(secretPrompt({ id: '5', request: secRequest }));
    vi.mocked(api.prompts.respondSecret).mockResolvedValue();
    await usePrompts.getState().respondSecret('5', 'hunter2', false);
    expect(api.prompts.respondSecret).toHaveBeenCalledWith('5', 'hunter2', false);
  });

  it('closes the dialog even when the backend no longer knows the prompt', async () => {
    usePrompts.getState().enqueue(hostKeyPrompt({ id: '1', request: hkRequest }));
    vi.mocked(api.prompts.respondHostKey).mockRejectedValue(new Error('unknown prompt'));
    await usePrompts.getState().respondHostKey('1', true);
    expect(ids()).toEqual([]);
  });
});
