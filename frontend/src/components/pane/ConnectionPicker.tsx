import { targets } from '../../api';
import type { Target } from '../../api/types';
import { sortByLabel } from '../../lib/sort';
import { useConnections } from '../../stores/connections';
import type { PaneId } from '../../stores/paneTypes';
import { usePanes } from '../../stores/panes';
import { useUi } from '../../stores/ui';
import { Select } from '../ui/Select';
import styles from './PaneHeader.module.css';

const QUICK = '__quick';
const MANAGE_S3 = '__s3';
const SSH_UNAVAILABLE = '__ssh-unavailable';

export function ConnectionPicker({ paneId }: { paneId: PaneId }) {
  const connId = usePanes((s) => s.panes[paneId].connId);
  const lastLabel = usePanes((s) => s.panes[paneId].lastTarget?.label);
  const { connections, hosts, profiles, sshUnavailable } = useConnections();
  const openDialog = useUi((s) => s.openDialog);

  // Option values are connection ids, so the select always shows the pane's real connection.
  const currentKey = connId ?? 'local';
  const known = new Set([
    'local',
    ...hosts.map((h) => `ssh:${h.alias}`),
    ...profiles.map((p) => `s3:${p.id}`),
  ]);
  const unlisted = known.has(currentKey)
    ? null
    : (connections.find((c) => c.id === currentKey)?.label ?? lastLabel ?? currentKey);

  const choose = (value: string) => {
    const connect = (target: Target, label: string) =>
      void usePanes.getState().connectTo(paneId, target, label);

    if (value === QUICK) return openDialog({ kind: 'quickConnect', paneId });
    if (value === MANAGE_S3) return openDialog({ kind: 's3Profiles' });
    if (value === 'local') return connect(targets.local(), 'Local');
    if (value.startsWith('ssh:')) {
      const alias = value.slice('ssh:'.length);
      return connect(targets.ssh(alias), alias);
    }
    if (value.startsWith('s3:')) {
      const id = value.slice('s3:'.length);
      const profile = profiles.find((p) => p.id === id);
      return connect(targets.s3(id), profile?.name ?? 'S3');
    }
  };

  return (
    <Select
      className={styles.picker}
      aria-label="Connection"
      value={currentKey}
      onChange={(e) => choose(e.target.value)}
    >
      <option value="local">Local</option>
      {unlisted !== null && <option value={currentKey}>{unlisted}</option>}
      {(hosts.length > 0 || sshUnavailable !== '') && (
        <optgroup label="SSH hosts">
          {sshUnavailable !== '' && (
            <option value={SSH_UNAVAILABLE} disabled>
              {sshUnavailable}
            </option>
          )}
          {sortByLabel(hosts, (h) => h.alias).map((h) => (
            <option key={h.alias} value={`ssh:${h.alias}`}>
              {h.alias}
            </option>
          ))}
        </optgroup>
      )}
      {profiles.length > 0 && (
        <optgroup label="S3">
          {sortByLabel(profiles, (p) => p.name).map((p) => (
            <option key={p.id} value={`s3:${p.id}`}>
              {p.name}
            </option>
          ))}
        </optgroup>
      )}
      <optgroup label="Other">
        <option value={QUICK}>Quick connect…</option>
        <option value={MANAGE_S3}>Manage S3 profiles…</option>
      </optgroup>
    </Select>
  );
}
