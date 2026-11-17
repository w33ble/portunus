import { useEffect, useState } from 'react';
import * as api from '../../api';
import { toAppError } from '../../api/errors';
import type { SSHHostDetails } from '../../api/types';
import { quickConnectLabel } from '../../lib/targets';
import controls from '../ui/controls.module.css';
import styles from './ConnectionsDialog.module.css';

type Load =
  | { status: 'loading' }
  | { status: 'ready'; details: SSHHostDetails }
  | { status: 'error'; message: string };

/** What `ssh -G` resolves for a ~/.ssh/config alias. Read-only: the file is the source. */
export function ConfigHostDetails({ alias }: { alias: string }) {
  const [load, setLoad] = useState<Load>({ status: 'loading' });

  useEffect(() => {
    let current = true;
    api.connections.sshHostDetails(alias).then(
      (details) => current && setLoad({ status: 'ready', details }),
      (e) => current && setLoad({ status: 'error', message: toAppError(e).message })
    );
    return () => {
      current = false;
    };
  }, [alias]);

  return (
    <div className={styles.fields}>
      <div className={controls.label}>
        Resolved by ssh
        {load.status === 'loading' && (
          <span className={controls.hint}>Reading the ssh configuration…</span>
        )}
        {load.status === 'error' && <span className={controls.errorText}>{load.message}</span>}
        {load.status === 'ready' && (
          <div className={styles.readonly}>
            <span className={controls.mono}>
              {quickConnectLabel(load.details.user, load.details.hostName, load.details.port)}
            </span>
            {load.details.proxyJump !== '' && (
              <span className={controls.mono}>ProxyJump {load.details.proxyJump}</span>
            )}
          </div>
        )}
      </div>
      <p className={controls.hint}>This host comes from ~/.ssh/config, so edit it there.</p>
    </div>
  );
}
