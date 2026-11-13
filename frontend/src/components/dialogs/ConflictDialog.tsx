import { useState } from 'react';
import { ConflictAction } from '../../api/enums';
import type { Entry } from '../../api/types';
import { formatBytes, formatDateTime } from '../../lib/format';
import type { Prompt } from '../../lib/prompts';
import { usePrompts } from '../../stores/prompts';
import controls from '../ui/controls.module.css';
import { Modal } from '../ui/Modal';
import styles from './PromptDialogs.module.css';

type Conflict = Extract<Prompt, { kind: 'conflict' }>;

function Side({ heading, entry }: { heading: string; entry: Entry }) {
  return (
    <div className={styles.side}>
      <h3>{heading}</h3>
      <p className={styles.sideName}>{entry.name}</p>
      <p className={controls.hint}>{entry.isDir ? 'Folder' : formatBytes(entry.size)}</p>
      <p className={controls.hint}>{formatDateTime(entry.modTime)}</p>
    </div>
  );
}

export function ConflictDialog({ prompt }: { prompt: Conflict }) {
  const respond = usePrompts((s) => s.respondConflict);
  const [applyToAll, setApplyToAll] = useState(false);
  const { src, dst } = prompt.request;

  const answer = (action: ConflictAction) => void respond(prompt.id, action, applyToAll);

  return (
    <Modal
      title={`“${dst.name}” already exists`}
      width={480}
      onCancel={() => answer(ConflictAction.ConflictCancel)}
      onSubmit={() => answer(ConflictAction.ConflictOverwrite)}
      actions={
        <>
          <button
            type="button"
            className={`${controls.button} ${controls.danger}`}
            onClick={() => answer(ConflictAction.ConflictCancel)}
          >
            Cancel job
          </button>
          <button
            type="button"
            className={controls.button}
            onClick={() => answer(ConflictAction.ConflictSkip)}
          >
            Skip
          </button>
          <button type="submit" className={`${controls.button} ${controls.primary}`}>
            Overwrite
          </button>
        </>
      }
    >
      <div className={styles.compare}>
        <Side heading="Incoming" entry={src} />
        <Side heading="Existing" entry={dst} />
      </div>
      <label className={controls.checkbox}>
        <input
          type="checkbox"
          checked={applyToAll}
          onChange={(e) => setApplyToAll(e.target.checked)}
        />
        Apply to all conflicts in this transfer
      </label>
    </Modal>
  );
}
