import { useEffect, useId, useState } from 'react';
import * as api from '../../api';
import type { Settings, VersionInfo } from '../../api/types';
import { IS_MAC } from '../../lib/platform';
import { useSettings } from '../../stores/settings';
import controls from '../ui/controls.module.css';
import { Modal } from '../ui/Modal';
import styles from './SettingsDialog.module.css';
import { ThemePicker } from './ThemePicker';
import { TransferLimitsFields } from './TransferLimitsFields';

type Section = 'general' | 'appearance';

const SECTIONS: ReadonlyArray<{ id: Section; label: string }> = [
  { id: 'general', label: 'General' },
  { id: 'appearance', label: 'Appearance' },
];

export function SettingsDialog({ onClose }: { onClose: () => void }) {
  const settings = useSettings((s) => s.settings);
  const [section, setSection] = useState<Section>('general');
  const [build, setBuild] = useState<VersionInfo | null>(null);
  const baseId = useId();

  useEffect(() => {
    api.version.info().then(setBuild, () => setBuild(null));
  }, []);

  return (
    <Modal
      title="Settings"
      width={500}
      onCancel={onClose}
      onSubmit={onClose}
      actions={
        <button type="submit" className={`${controls.button} ${controls.primary}`}>
          Done
        </button>
      }
    >
      <div role="tablist" aria-label="Settings sections" className={styles.tabs}>
        {SECTIONS.map(({ id, label }) => (
          <button
            key={id}
            type="button"
            role="tab"
            id={`${baseId}-${id}-tab`}
            aria-controls={`${baseId}-${id}`}
            aria-selected={section === id}
            className={styles.tab}
            onClick={() => setSection(id)}
          >
            {label}
          </button>
        ))}
      </div>
      <div
        role="tabpanel"
        id={`${baseId}-${section}`}
        aria-labelledby={`${baseId}-${section}-tab`}
        className={styles.panel}
      >
        {settings === null ? (
          <p className={controls.hint}>Loading…</p>
        ) : section === 'general' ? (
          <GeneralSection settings={settings} />
        ) : (
          <ThemePicker />
        )}
      </div>
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

function GeneralSection({ settings }: { settings: Settings }) {
  const save = useSettings((s) => s.save);

  return (
    <>
      <label className={controls.checkbox}>
        <input
          type="checkbox"
          checked={settings.showHidden}
          onChange={(e) => void save({ showHidden: e.target.checked })}
        />
        Show hidden files
      </label>
      <div>
        <label className={controls.checkbox}>
          <input
            type="checkbox"
            checked={settings.swipeNavigation}
            onChange={(e) => void save({ swipeNavigation: e.target.checked })}
          />
          Swipe to go back and forward
        </label>
        <p className={controls.hint}>
          {IS_MAC
            ? 'Uses the gesture chosen for “Swipe between pages” in System Settings → Trackpad.'
            : 'A two-finger sideways swipe on the touchpad over a pane moves through its history.'}
        </p>
      </div>
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
  );
}
