import { useState } from 'react';
import type { Prompt } from '../../lib/prompts';
import { usePrompts } from '../../stores/prompts';
import controls from '../ui/controls.module.css';
import { Modal } from '../ui/Modal';

type Secret = Extract<Prompt, { kind: 'secret' }>;

export function SecretDialog({ prompt }: { prompt: Secret }) {
  const respond = usePrompts((s) => s.respondSecret);
  const [value, setValue] = useState('');
  const { title, message, label, echo } = prompt.request;

  return (
    <Modal
      title={title}
      width={400}
      onCancel={() => void respond(prompt.id, '', true)}
      onSubmit={() => void respond(prompt.id, value, false)}
      actions={
        <>
          <button
            type="button"
            className={controls.button}
            onClick={() => void respond(prompt.id, '', true)}
          >
            Cancel
          </button>
          <button type="submit" className={`${controls.button} ${controls.primary}`}>
            OK
          </button>
        </>
      }
    >
      {message !== '' && <p className={controls.hint}>{message}</p>}
      <label className={controls.label}>
        {label}
        <input
          className={controls.field}
          type={echo ? 'text' : 'password'}
          value={value}
          autoFocus
          autoComplete="off"
          spellCheck={false}
          autoCapitalize="off"
          autoCorrect="off"
          onChange={(e) => setValue(e.target.value)}
        />
      </label>
    </Modal>
  );
}
