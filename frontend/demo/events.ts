// Stands in for src/api/events.ts in the demo build: nothing is ever pushed.
import type * as real from '../src/api/events';

const never = () => () => undefined;

export const events: typeof real.events = {
  transfersUpdate: never,
  connectionsChanged: never,
  dropFiles: never,
  promptHostKey: never,
  promptSecret: never,
  promptConflict: never,
  promptResolved: never,
  swipe: never,
};
