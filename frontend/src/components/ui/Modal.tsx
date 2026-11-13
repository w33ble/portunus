import { type FormEvent, type ReactNode, useEffect, useId, useRef } from 'react';
import styles from './Modal.module.css';

interface ModalProps {
  title: string;
  children: ReactNode;
  /** Buttons, rightmost last. */
  actions: ReactNode;
  /** Called on Escape. Omit to make the dialog undismissable by keyboard. */
  onCancel?: () => void;
  /** Called on Enter / the submit button. */
  onSubmit?: () => void;
  width?: number;
}

/** A native modal <dialog>: focus trap, Escape, and inert background come from the platform. */
export function Modal({ title, children, actions, onCancel, onSubmit, width = 420 }: ModalProps) {
  const ref = useRef<HTMLDialogElement>(null);
  const titleId = useId();

  useEffect(() => {
    const dialog = ref.current;
    if (dialog && !dialog.open) dialog.showModal();
    return () => dialog?.close();
  }, []);

  const submit = (event: FormEvent) => {
    event.preventDefault();
    onSubmit?.();
  };

  return (
    <dialog
      ref={ref}
      className={styles.dialog}
      style={{ width }}
      aria-labelledby={titleId}
      onCancel={(event) => {
        event.preventDefault();
        onCancel?.();
      }}
    >
      <form className={styles.form} onSubmit={submit}>
        <h2 id={titleId} className={styles.title}>
          {title}
        </h2>
        <div className={styles.body}>{children}</div>
        <div className={styles.actions}>{actions}</div>
      </form>
    </dialog>
  );
}
