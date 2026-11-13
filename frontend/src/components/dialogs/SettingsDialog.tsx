import { useSettings } from '../../stores/settings';
import controls from '../ui/controls.module.css';
import { Modal } from '../ui/Modal';
import { TransferLimitsFields } from './TransferLimitsFields';

const IS_MAC = typeof navigator !== 'undefined' && /Mac/i.test(navigator.platform);

export function SettingsDialog({ onClose }: { onClose: () => void }) {
  const settings = useSettings((s) => s.settings);
  const save = useSettings((s) => s.save);

  return (
    <Modal
      title="Settings"
      onCancel={onClose}
      onSubmit={onClose}
      actions={
        <button type="submit" className={`${controls.button} ${controls.primary}`}>
          Done
        </button>
      }
    >
      {settings === null ? (
        <p className={controls.hint}>Loading…</p>
      ) : (
        <>
          <label className={controls.checkbox}>
            <input
              type="checkbox"
              checked={settings.showHidden}
              onChange={(e) => void save({ showHidden: e.target.checked })}
            />
            Show hidden files
          </label>
          {IS_MAC && (
            <div>
              <label className={controls.checkbox}>
                <input
                  type="checkbox"
                  checked={settings.loadAppleKeychain}
                  onChange={(e) => void save({ loadAppleKeychain: e.target.checked })}
                />
                Load SSH keys from the macOS keychain
              </label>
              <p className={controls.hint}>
                Runs <code>ssh-add --apple-load-keychain</code> before asking for a key passphrase.
              </p>
            </div>
          )}
          <TransferLimitsFields />
        </>
      )}
    </Modal>
  );
}
