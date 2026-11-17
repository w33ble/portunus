import { useCallback, useRef, useState } from 'react';
import { EMPTY_PREFS, type KnownConnection, LOCAL_ID } from '../../lib/catalog';
import { useCatalog, useConnections } from '../../stores/connections';
import type { PaneId } from '../../stores/paneTypes';
import { usePanes } from '../../stores/panes';
import controls from '../ui/controls.module.css';
import { Icon } from '../ui/Icon';
import { ColorDot } from './ColorDot';
import styles from './ConnectionPicker.module.css';
import { ConnectionPickerPopover } from './ConnectionPickerPopover';

/** The connection the pane shows: its catalog entry, else what the pane last connected to. */
function usePaneConnection(
  paneId: PaneId,
  catalog: readonly KnownConnection[]
): KnownConnection | null {
  const connId = usePanes((s) => s.panes[paneId].connId);
  const lastTarget = usePanes((s) => s.panes[paneId].lastTarget);
  const live = useConnections((s) => s.connections.find((c) => c.id === connId));
  const prefs = useConnections((s) => s.prefs[connId ?? LOCAL_ID]);

  const id = connId ?? LOCAL_ID;
  const known = catalog.find((c) => c.id === id);
  if (known) return known;

  const target = live?.target ?? lastTarget?.target;
  const name = live?.label ?? lastTarget?.label;
  if (!target || name === undefined) return null;
  return {
    id,
    source: id.startsWith('s3:') ? 's3' : 'saved',
    name,
    target,
    prefs: prefs ?? EMPTY_PREFS,
  };
}

/** Pane header button that opens a filterable, grouped list of connections. */
export function ConnectionPicker({ paneId }: { paneId: PaneId }) {
  const catalog = useCatalog();
  const current = usePaneConnection(paneId, catalog);
  const [anchor, setAnchor] = useState<{ x: number; y: number } | null>(null);
  const trigger = useRef<HTMLButtonElement>(null);

  const open = () => {
    const rect = trigger.current?.getBoundingClientRect();
    if (rect) setAnchor({ x: rect.left, y: rect.bottom + 4 });
  };

  const close = useCallback((refocus: boolean) => {
    setAnchor(null);
    if (refocus) trigger.current?.focus();
  }, []);

  return (
    <>
      <button
        ref={trigger}
        type="button"
        className={`${controls.field} ${styles.trigger}`}
        aria-label="Connection"
        aria-haspopup="listbox"
        aria-expanded={anchor !== null}
        onClick={() => (anchor === null ? open() : close(false))}
        onKeyDown={(e) => {
          if (e.key === 'ArrowDown' && anchor === null) {
            e.preventDefault();
            open();
          }
        }}
      >
        {current?.prefs.color && <ColorDot color={current.prefs.color} />}
        <span className={styles.name}>{current?.name ?? 'Local'}</span>
        <Icon name="chevronDown" size={12} className={styles.chevron} />
      </button>
      {anchor && (
        <ConnectionPickerPopover
          paneId={paneId}
          catalog={catalog}
          current={current}
          anchor={anchor}
          trigger={trigger}
          onClose={close}
        />
      )}
    </>
  );
}
