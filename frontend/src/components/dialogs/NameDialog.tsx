import { useRef, useState } from 'react';
import { toAppError } from '../../api/errors';
import controls from '../ui/controls.module.css';
import { Modal } from '../ui/Modal';

interface Props {
  title: string;
  label: string;
  confirmLabel: string;
  initial: string;
  /** Throw to keep the dialog open and show the message. */
  onSubmit: (name: string) => Promise<void>;
  onClose: () => void;
}

export function NameDialog({ title, label, confirmLabel, initial, onSubmit, onClose }: Props) {
  const [name, setName] = useState(initial);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const input = useRef<HTMLInputElement>(null);

  const submit = async () => {
    const trimmed = name.trim();
    if (trimmed === '' || trimmed === initial) {
      if (trimmed === initial && initial !== '') onClose();
      return;
    }
    setBusy(true);
    try {
      await onSubmit(trimmed);
      onClose();
    } catch (e) {
      setError(toAppError(e).message);
      setBusy(false);
      input.current?.select();
    }
  };

  return (
    <Modal
      title={title}
      width={380}
      onCancel={onClose}
      onSubmit={() => void submit()}
      actions={
        <>
          <button type="button" className={controls.button} onClick={onClose}>
            Cancel
          </button>
          <button
            type="submit"
            className={`${controls.button} ${controls.primary}`}
            disabled={busy || name.trim() === ''}
          >
            {confirmLabel}
          </button>
        </>
      }
    >
      <label className={controls.label}>
        {label}
        <input
          ref={input}
          className={controls.field}
          value={name}
          autoFocus
          spellCheck={false}
          autoCapitalize="off"
          autoCorrect="off"
          onChange={(e) => {
            setName(e.target.value);
            setError(null);
          }}
          onFocus={(e) => {
            // Select the stem so a rename keeps the extension unless the user wants it gone.
            const dot = e.target.value.lastIndexOf('.');
            e.target.setSelectionRange(0, dot > 0 ? dot : e.target.value.length);
          }}
        />
      </label>
      {error && <p className={controls.errorText}>{error}</p>}
    </Modal>
  );
}
