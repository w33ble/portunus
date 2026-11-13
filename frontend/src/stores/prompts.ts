import { create } from 'zustand';
import * as api from '../api';
import type { ConflictAction } from '../api/enums';
import { toAppError } from '../api/errors';
import {
  type Prompt,
  type PromptQueue,
  enqueue,
  mergePending,
  resolvePrompt,
} from '../lib/prompts';
import { useUi } from './ui';

interface PromptsState {
  queue: PromptQueue;
  init: () => Promise<void>;
  enqueue: (prompt: Prompt) => void;
  resolve: (id: string) => void;
  respondHostKey: (id: string, accept: boolean) => Promise<void>;
  respondSecret: (id: string, value: string, cancel: boolean) => Promise<void>;
  respondConflict: (id: string, action: ConflictAction, applyToAll: boolean) => Promise<void>;
}

export const usePrompts = create<PromptsState>()((set, get) => {
  /** The dialog closes either way; a stale id (answered elsewhere) is not worth an error. */
  async function answer(id: string, send: () => Promise<void>) {
    try {
      await send();
    } catch (e) {
      const error = toAppError(e);
      if (error.kind !== 'canceled') {
        useUi.getState().showError('Couldn’t send your answer', error);
      }
    }
    get().resolve(id);
  }

  return {
    queue: [],

    async init() {
      try {
        const pending = await api.prompts.pending();
        set((s) => ({ queue: mergePending(s.queue, pending) }));
      } catch (e) {
        useUi.getState().showError('Couldn’t load pending prompts', toAppError(e));
      }
    },

    enqueue: (prompt) => set((s) => ({ queue: enqueue(s.queue, prompt) })),
    resolve: (id) => set((s) => ({ queue: resolvePrompt(s.queue, id) })),

    respondHostKey: (id, accept) => answer(id, () => api.prompts.respondHostKey(id, accept)),
    respondSecret: (id, value, cancel) =>
      answer(id, () => api.prompts.respondSecret(id, value, cancel)),
    respondConflict: (id, action, applyToAll) =>
      answer(id, () => api.prompts.respondConflict(id, { action, applyToAll })),
  };
});
