import { useEffect, useState } from 'react';
import * as api from '../../api';
import type { VersionInfo } from '../../api/types';
import { IS_MAC } from '../../lib/platform';
import { useSettings } from '../../stores/settings';
import controls from '../ui/controls.module.css';
import { Modal } from '../ui/Modal';
import { TransferLimitsFields } from './TransferLimitsFields';

export function SettingsDialog({ onClose }: { onClose: () => void }) {
  const settings = useSettings((s) => s.settings);
  const save = useSettings((s) => s.save);
  const [build, setBuild] = useState<VersionInfo | null>(null);

  useEffect(() => {
    api.version.info().then(setBuild, () => setBuild(null));
  }, []);

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
      {build !== null && (
        <p className={controls.hint}>
          Portunus {build.version}
          {build.commit !== '' &&
            !build.version.includes(build.commit) &&
            ` (${build.commit})`} ·{' '}
          {build.go}
        </p>
      )}
    </Modal>
  );
}
