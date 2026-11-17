import { create } from 'zustand';
import type { AppError } from '../api/errors';
import type { Bookmark } from '../api/types';
import type { PaneId } from './paneTypes';

export type Tab = 'browser' | 'transfers';

export interface DeleteTarget {
  path: string;
  name: string;
  isDir: boolean;
}

export type Dialog =
  | { kind: 'confirmDelete'; paneId: PaneId; connId: string; targets: DeleteTarget[] }
  | { kind: 'newFolder'; paneId: PaneId }
  | { kind: 'rename'; paneId: PaneId; path: string; name: string }
  | { kind: 'quickConnect'; paneId: PaneId }
  /** The connection manager, with `connId` selected (else the first connection). */
  | { kind: 'connections'; connId?: string }
  /** Asks before deleting a saved bookmark (⌘D or the bookmarks menu on a bookmarked folder). */
  | { kind: 'removeBookmark'; bookmark: Bookmark }
  | { kind: 'settings' }
  | { kind: 'error'; title: string; message: string; hint: string | null };

const HOST_KEY_HINT =
  'The server presented a different host key than the one saved in known_hosts. This can mean the ' +
  'server was reinstalled, or that someone is intercepting the connection. Portunus won’t connect ' +
  'until you resolve it: verify the new key with the server’s administrator, then remove the old ' +
  'entry from your known_hosts file (the message above names the file and line).';

interface UiState {
  tab: Tab;
  activePane: PaneId;
  dialog: Dialog | null;
  /** Pane whose cursor entry is previewed in place of the other pane; null when closed. */
  preview: PaneId | null;
  setTab: (tab: Tab) => void;
  setActivePane: (id: PaneId) => void;
  openDialog: (dialog: Dialog) => void;
  closeDialog: () => void;
  /** Space in a file list: opens the preview for `id`, or closes it. */
  togglePreview: (id: PaneId) => void;
  closePreview: () => void;
  showError: (title: string, error: AppError) => void;
}

export const useUi = create<UiState>()((set) => ({
  tab: 'browser',
  activePane: 'left',
  dialog: null,
  preview: null,
  setTab: (tab) => set({ tab }),
  setActivePane: (activePane) => set({ activePane }),
  openDialog: (dialog) => set({ dialog }),
  closeDialog: () => set({ dialog: null }),
  togglePreview: (id) => set((s) => ({ preview: s.preview === id ? null : id })),
  closePreview: () => set({ preview: null }),
  showError(title, error) {
    if (error.kind === 'canceled') return;
    if (error.kind === 'hostkey_mismatch') {
      set({
        dialog: {
          kind: 'error',
          title: 'Host key changed',
          message: error.message,
          hint: HOST_KEY_HINT,
        },
      });
      return;
    }
    set({ dialog: { kind: 'error', title, message: error.message, hint: null } });
  },
}));
