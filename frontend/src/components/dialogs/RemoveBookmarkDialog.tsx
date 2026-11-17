import { useState } from 'react';
import { toAppError } from '../../api/errors';
import type { Bookmark } from '../../api/types';
import { bookmarkLabel } from '../../lib/catalog';
import { useConnections } from '../../stores/connections';
import controls from '../ui/controls.module.css';
import { Modal } from '../ui/Modal';

interface Props {
  bookmark: Bookmark;
  onClose: () => void;
}

export function RemoveBookmarkDialog({ bookmark, onClose }: Props) {
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const submit = async () => {
    setBusy(true);
    try {
      await useConnections.getState().removeBookmark(bookmark.id);
      onClose();
    } catch (e) {
      setError(toAppError(e).message);
      setBusy(false);
    }
  };

  return (
    <Modal
      title="Remove bookmark?"
      width={380}
      onCancel={onClose}
      onSubmit={() => void submit()}
      actions={
        <>
          <button type="button" className={controls.button} onClick={onClose} autoFocus>
            Cancel
          </button>
          <button type="submit" className={`${controls.button} ${controls.danger}`} disabled={busy}>
            Remove
          </button>
        </>
      }
    >
      <p className={controls.hint}>
        “{bookmarkLabel(bookmark)}” (<span className={controls.mono}>{bookmark.path}</span>) will no
        longer be bookmarked. The folder itself isn’t touched.
      </p>
      {error && <p className={controls.errorText}>{error}</p>}
    </Modal>
  );
}
