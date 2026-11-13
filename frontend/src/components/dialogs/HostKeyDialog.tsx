import type { Prompt } from '../../lib/prompts';
import { usePrompts } from '../../stores/prompts';
import controls from '../ui/controls.module.css';
import { Modal } from '../ui/Modal';
import styles from './PromptDialogs.module.css';

type HostKey = Extract<Prompt, { kind: 'hostkey' }>;

export function HostKeyDialog({ prompt }: { prompt: HostKey }) {
  const respond = usePrompts((s) => s.respondHostKey);
  const { host, address, keyType, fingerprint, saveTo } = prompt.request;

  return (
    <Modal
      title={`Trust the host key for ${host}?`}
      width={460}
      onCancel={() => void respond(prompt.id, false)}
      onSubmit={() => void respond(prompt.id, true)}
      actions={
        <>
          <button
            type="button"
            className={controls.button}
            onClick={() => void respond(prompt.id, false)}
            autoFocus
          >
            Cancel
          </button>
          <button type="submit" className={`${controls.button} ${controls.primary}`}>
            Trust
          </button>
        </>
      }
    >
      <p className={controls.hint}>
        This is the first time you’re connecting to this server. Make sure the fingerprint matches
        what the server’s administrator gave you.
      </p>
      <dl className={styles.facts}>
        <dt>Host</dt>
        <dd>{host}</dd>
        <dt>Address</dt>
        <dd className={controls.mono}>{address}</dd>
        <dt>Key type</dt>
        <dd className={controls.mono}>{keyType}</dd>
        <dt>Fingerprint</dt>
        <dd className={controls.mono}>{fingerprint}</dd>
        <dt>Saved to</dt>
        <dd className={controls.mono}>{saveTo}</dd>
      </dl>
    </Modal>
  );
}
