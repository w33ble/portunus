import type { ConflictPrompt, HostKeyPrompt, Pending, SecretPrompt } from '../api/types';

export type Prompt =
  | { kind: 'hostkey'; id: string; request: HostKeyPrompt['request'] }
  | { kind: 'secret'; id: string; request: SecretPrompt['request'] }
  | { kind: 'conflict'; id: string; request: ConflictPrompt['request'] };

export type PromptQueue = readonly Prompt[];

export function hostKeyPrompt(p: HostKeyPrompt): Prompt {
  return { kind: 'hostkey', id: p.id, request: p.request };
}
export function secretPrompt(p: SecretPrompt): Prompt {
  return { kind: 'secret', id: p.id, request: p.request };
}
export function conflictPrompt(p: ConflictPrompt): Prompt {
  return { kind: 'conflict', id: p.id, request: p.request };
}

/** Appends unless the id is already queued (an event can race Pending()). */
export function enqueue(queue: PromptQueue, prompt: Prompt): PromptQueue {
  return queue.some((p) => p.id === prompt.id) ? queue : [...queue, prompt];
}

export function resolvePrompt(queue: PromptQueue, id: string): PromptQueue {
  return queue.some((p) => p.id === id) ? queue.filter((p) => p.id !== id) : queue;
}

/** Prompt ids are monotonically increasing integers, so numeric order is arrival order. */
export function fromPending(pending: Pending): Prompt[] {
  return [
    ...(pending.hostKeys ?? []).map(hostKeyPrompt),
    ...(pending.secrets ?? []).map(secretPrompt),
    ...(pending.conflicts ?? []).map(conflictPrompt),
  ].toSorted((a, b) => Number(a.id) - Number(b.id));
}

/** Merges pending prompts into a queue, keeping arrival order and dropping duplicates. */
export function mergePending(queue: PromptQueue, pending: Pending): PromptQueue {
  const merged = fromPending(pending).reduce(enqueue, queue);
  return merged === queue ? queue : merged.toSorted((a, b) => Number(a.id) - Number(b.id));
}
