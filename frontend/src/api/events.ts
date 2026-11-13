import { Events } from '@wailsio/runtime';
import * as names from '../../bindings/events';
import type {
  ConflictPrompt,
  Connection,
  DropFiles,
  FilePage,
  HostKeyPrompt,
  SecretPrompt,
  Update,
} from './types';

type Unsubscribe = () => void;

export const events = {
  filesPage: (cb: (page: FilePage) => void): Unsubscribe =>
    Events.On(names.FilesPage, (ev) => cb(ev.data)),
  transfersUpdate: (cb: (update: Update) => void): Unsubscribe =>
    Events.On(names.TransfersUpdate, (ev) => cb(ev.data)),
  connectionsChanged: (cb: (list: Connection[]) => void): Unsubscribe =>
    Events.On(names.ConnectionsChanged, (ev) => cb(ev.data ?? [])),
  dropFiles: (cb: (drop: DropFiles) => void): Unsubscribe =>
    Events.On(names.DropFiles, (ev) => cb(ev.data)),
  promptHostKey: (cb: (p: HostKeyPrompt) => void): Unsubscribe =>
    Events.On(names.PromptHostkey, (ev) => cb(ev.data)),
  promptSecret: (cb: (p: SecretPrompt) => void): Unsubscribe =>
    Events.On(names.PromptSecret, (ev) => cb(ev.data)),
  promptConflict: (cb: (p: ConflictPrompt) => void): Unsubscribe =>
    Events.On(names.PromptConflict, (ev) => cb(ev.data)),
  promptResolved: (cb: (id: string) => void): Unsubscribe =>
    Events.On(names.PromptResolved, (ev) => cb(ev.data)),
};
