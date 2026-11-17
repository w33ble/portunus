import { pickerConnections } from '../../lib/catalog';
import { useCatalog, useConnections } from '../../stores/connections';
import type { PaneId } from '../../stores/paneTypes';
import { usePanes } from '../../stores/panes';
import { useUi } from '../../stores/ui';
import { Select } from '../ui/Select';
import styles from './PaneHeader.module.css';

const QUICK = '__quick';
const MANAGE = '__manage';
const SSH_UNAVAILABLE = '__ssh-unavailable';

export function ConnectionPicker({ paneId }: { paneId: PaneId }) {
  const connId = usePanes((s) => s.panes[paneId].connId);
  const lastLabel = usePanes((s) => s.panes[paneId].lastTarget?.label);
  const live = useConnections((s) => s.connections);
  const sshUnavailable = useConnections((s) => s.sshUnavailable);
  const openDialog = useUi((s) => s.openDialog);
  const catalog = useCatalog();

  const offered = pickerConnections(catalog);
  const ssh = offered.filter((c) => c.source === 'config' || c.source === 'saved');
  const s3 = offered.filter((c) => c.source === 's3');

  // Option values are connection ids, so the select always shows the pane's real connection.
  const currentKey = connId ?? 'local';
  const unlisted = offered.some((c) => c.id === currentKey)
    ? null
    : (catalog.find((c) => c.id === currentKey)?.name ??
      live.find((c) => c.id === currentKey)?.label ??
      lastLabel ??
      currentKey);

  const choose = (value: string) => {
    if (value === QUICK) return openDialog({ kind: 'quickConnect', paneId });
    if (value === MANAGE) {
      const known = connId !== null && catalog.some((c) => c.id === connId);
      return openDialog({ kind: 'connections', ...(known && { connId }) });
    }
    const conn = offered.find((c) => c.id === value);
    if (conn) void usePanes.getState().connectTo(paneId, conn.target, conn.name);
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
      {(ssh.length > 0 || sshUnavailable !== '') && (
        <optgroup label="SSH hosts">
          {sshUnavailable !== '' && (
            <option value={SSH_UNAVAILABLE} disabled>
              {sshUnavailable}
            </option>
          )}
          {ssh.map((c) => (
            <option key={c.id} value={c.id}>
              {c.name}
            </option>
          ))}
        </optgroup>
      )}
      {s3.length > 0 && (
        <optgroup label="S3">
          {s3.map((c) => (
            <option key={c.id} value={c.id}>
              {c.name}
            </option>
          ))}
        </optgroup>
      )}
      <optgroup label="Other">
        <option value={QUICK}>Quick connect…</option>
        <option value={MANAGE}>Manage connections…</option>
      </optgroup>
    </Select>
  );
}
