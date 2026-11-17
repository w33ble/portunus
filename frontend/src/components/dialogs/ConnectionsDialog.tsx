import controls from '../ui/controls.module.css';
import { Modal } from '../ui/Modal';
import { ConnectionDetail } from './ConnectionDetail';
import styles from './ConnectionsDialog.module.css';
import { ConnectionsSidebar } from './ConnectionsSidebar';
import { useConnectionEditor } from './useConnectionEditor';

interface Props {
  /** The connection to select first; Local when omitted. */
  connId?: string;
  onClose: () => void;
}

export function ConnectionsDialog({ connId, onClose }: Props) {
  const editor = useConnectionEditor(connId, onClose);
  const { dirty, busy, error } = editor;

  return (
    <>
      <Modal
        title="Connections"
        width={780}
        onCancel={editor.done}
        onSubmit={() => {
          if (busy) return;
          if (dirty) void editor.save();
          else editor.done();
        }}
        actions={
          <div className={styles.footer}>
            <div className={styles.footerGroup}>
              <button
                type="button"
                className={controls.button}
                disabled={!dirty || busy}
                onClick={editor.revert}
              >
                Revert
              </button>
              <button
                type={dirty ? 'submit' : 'button'}
                className={`${controls.button} ${dirty ? controls.primary : ''}`}
                disabled={!dirty || busy}
              >
                Save
              </button>
            </div>
            {error && (
              <p role="alert" className={`${controls.errorText} ${styles.footerError}`}>
                {error}
              </p>
            )}
            <button
              type={dirty ? 'button' : 'submit'}
              className={`${controls.button} ${dirty ? '' : controls.primary}`}
              onClick={dirty ? editor.done : undefined}
            >
              Done
            </button>
          </div>
        }
      >
        <div className={styles.split}>
          <ConnectionsSidebar editor={editor} />
          <ConnectionDetail key={editor.key} editor={editor} />
        </div>
      </Modal>
      {editor.asking && (
        <Modal
          title={`Save changes to ${editor.label}?`}
          width={380}
          onCancel={() => void editor.answer('cancel')}
          onSubmit={() => void editor.answer('save')}
          actions={
            <>
              <button
                type="button"
                className={controls.button}
                onClick={() => void editor.answer('discard')}
              >
                Don’t save
              </button>
              <button
                type="button"
                className={controls.button}
                onClick={() => void editor.answer('cancel')}
              >
                Cancel
              </button>
              <button type="submit" className={`${controls.button} ${controls.primary}`}>
                Save
              </button>
            </>
          }
        >
          <p className={controls.hint}>Your changes will be lost if you don’t save them.</p>
        </Modal>
      )}
    </>
  );
}
