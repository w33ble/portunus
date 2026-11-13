import { useState } from 'react';
import { targets } from '../../api';
import { parseQuickConnect } from '../../lib/quickConnect';
import { quickConnectLabel } from '../../lib/targets';
import type { PaneId } from '../../stores/paneTypes';
import { usePanes } from '../../stores/panes';
import controls from '../ui/controls.module.css';
import { Modal } from '../ui/Modal';

interface Props {
  paneId: PaneId;
  onClose: () => void;
}

export function QuickConnectDialog({ paneId, onClose }: Props) {
  const [text, setText] = useState('');
  const [error, setError] = useState<string | null>(null);

  const submit = () => {
    const parsed = parseQuickConnect(text);
    if (!parsed.ok) {
      setError(parsed.error);
      return;
    }
    const { user, host, port } = parsed.target;
    onClose();
    void usePanes
      .getState()
      .connectTo(paneId, targets.quick(user, host, port), quickConnectLabel(user, host, port));
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
