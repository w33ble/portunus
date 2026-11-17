import { useState } from 'react';
import { targets } from '../../api';
import { toAppError } from '../../api/errors';
import { parseQuickConnect } from '../../lib/quickConnect';
import { quickConnectLabel } from '../../lib/targets';
import { useConnections } from '../../stores/connections';
import type { PaneId } from '../../stores/paneTypes';
import { usePanes } from '../../stores/panes';
import { useUi } from '../../stores/ui';
import controls from '../ui/controls.module.css';
import { Modal } from '../ui/Modal';

interface Props {
  paneId: PaneId;
  onClose: () => void;
}

export function QuickConnectDialog({ paneId, onClose }: Props) {
  const [text, setText] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [save, setSave] = useState(false);
  const [name, setName] = useState<string | null>(null);

  const parsed = parseQuickConnect(text);
  const label = parsed.ok
    ? quickConnectLabel(parsed.target.user, parsed.target.host, parsed.target.port)
    : '';
  const hostName = name ?? label;

  const submit = () => {
    if (!parsed.ok) {
      setError(parsed.error);
      return;
    }
    const { user, host, port } = parsed.target;
    const saveAs = hostName.trim();
    onClose();
    void (async () => {
      const connected = await usePanes
        .getState()
        .connectTo(paneId, targets.quick(user, host, port), label);
      if (!connected || !save) return;
      try {
        await useConnections
          .getState()
          .saveHost({ id: '', name: saveAs || label, user, host, port, connId: '' });
      } catch (e) {
        useUi.getState().showError(`Couldn’t save ${label}`, toAppError(e));
      }
    })();
  };

  return (
    <Modal
      title="Quick connect"
      width={400}
      onCancel={onClose}
      onSubmit={submit}
      actions={
        <>
          <button type="button" className={controls.button} onClick={onClose}>
            Cancel
          </button>
          <button
            type="submit"
            className={`${controls.button} ${controls.primary}`}
            disabled={text.trim() === ''}
          >
            Connect
          </button>
        </>
      }
    >
      <label className={controls.label}>
        Server (SFTP over SSH)
        <input
          className={controls.field}
          value={text}
          placeholder="user@host:port"
          autoFocus
          spellCheck={false}
          autoCapitalize="off"
          autoCorrect="off"
          onChange={(e) => {
            setText(e.target.value);
            setError(null);
          }}
        />
      </label>
      <label className={controls.checkbox}>
        <input type="checkbox" checked={save} onChange={(e) => setSave(e.target.checked)} />
        Save as a host
      </label>
      {save && (
        <label className={controls.label}>
          Name
          <input
            className={controls.field}
            value={hostName}
            spellCheck={false}
            onChange={(e) => setName(e.target.value)}
          />
        </label>
      )}
      {error ? (
        <p className={controls.errorText}>{error}</p>
      ) : (
        <p className={controls.hint}>
          User and port are optional. Settings from ~/.ssh/config still apply to the host name.
        </p>
      )}
    </Modal>
  );
}
