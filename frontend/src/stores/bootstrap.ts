import { events } from '../api/events';
import { conflictPrompt, hostKeyPrompt, secretPrompt } from '../lib/prompts';
import { dropFromOs } from './fileOps';
import { useConnections } from './connections';
import { parsePaneId } from './paneTypes';
import { receivePage, usePanes } from './panes';
import { usePrompts } from './prompts';
import { useSettings } from './settings';
import { useTransfers } from './transfers';
import { useUi } from './ui';

/** Subscribes to backend events, loads initial state, and opens the panes. Returns the cleanup. */
export function startBackend(): () => void {
  const off = [
    events.filesPage((page) => receivePage(page.requestId, page.entries ?? [])),
    events.transfersUpdate((update) => useTransfers.getState().apply(update)),
    events.connectionsChanged((list) => useConnections.getState().setConnections(list)),
    events.dropFiles((drop) => {
      const target = parsePaneId(drop.pane) ?? useUi.getState().activePane;
      dropFromOs(target, drop.paths ?? [], drop.dir);
    }),
    events.promptHostKey((p) => usePrompts.getState().enqueue(hostKeyPrompt(p))),
    events.promptSecret((p) => usePrompts.getState().enqueue(secretPrompt(p))),
    events.promptConflict((p) => usePrompts.getState().enqueue(conflictPrompt(p))),
    events.promptResolved((id) => usePrompts.getState().resolve(id)),
  ];

  // Listeners are live first, so nothing emitted during the initial fetches is lost.
  // Panes open independently: a slow or hung call (e.g. reading ~/.ssh/config) must not
  // leave them blank.
  usePanes.getState().init();
  void useSettings.getState().load();
  void useConnections.getState().load();
  void useTransfers.getState().init();
  void usePrompts.getState().init();

  return () => {
    for (const unsubscribe of off) unsubscribe();
  };
}
