import controls from '../ui/controls.module.css';
import { Modal } from '../ui/Modal';

interface Props {
  title: string;
  message: string;
  hint: string | null;
  onClose: () => void;
}

export function ErrorDialog({ title, message, hint, onClose }: Props) {
  return (
    <Modal
      title={title}
      onCancel={onClose}
      onSubmit={onClose}
      actions={
        <button type="submit" className={`${controls.button} ${controls.primary}`} autoFocus>
          OK
        </button>
      }
    >
      <p className={controls.mono}>{message}</p>
      {hint && <p className={controls.hint}>{hint}</p>}
    </Modal>
  );
}
