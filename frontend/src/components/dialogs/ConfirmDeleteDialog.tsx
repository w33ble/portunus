import { confirmDelete } from '../../stores/fileOps';
import type { DeleteTarget } from '../../stores/ui';
import controls from '../ui/controls.module.css';
import { Modal } from '../ui/Modal';

interface Props {
  connId: string;
  targets: DeleteTarget[];
  onClose: () => void;
}

const LISTED = 5;

function heading(targets: DeleteTarget[]): string {
  const [only] = targets;
  if (targets.length === 1 && only) return `Delete “${only.name}”?`;
  return `Delete ${targets.length} items?`;
}

export function ConfirmDeleteDialog({ connId, targets, onClose }: Props) {
  const hasDir = targets.some((t) => t.isDir);

  const submit = () => {
    onClose();
    void confirmDelete(
      connId,
      targets.map((t) => t.path)
    );
  };

  return (
    <Modal
      title={heading(targets)}
      onCancel={onClose}
      onSubmit={submit}
      actions={
        <>
          <button type="button" className={controls.button} onClick={onClose} autoFocus>
            Cancel
          </button>
          <button type="submit" className={`${controls.button} ${controls.primary}`}>
            Delete
          </button>
        </>
      }
    >
      {targets.length > 1 && (
        <ul className={controls.hint}>
          {targets.slice(0, LISTED).map((t) => (
            <li key={t.path}>{t.name}</li>
          ))}
          {targets.length > LISTED && <li>and {targets.length - LISTED} more…</li>}
        </ul>
      )}
      <p className={controls.hint}>
        {hasDir ? 'Folders are deleted with everything inside them. ' : ''}
        This can’t be undone.
      </p>
    </Modal>
  );
}
