import { type KeyboardEvent, type RefObject, useEffect, useId, useRef, useState } from 'react';
import type { KnownConnection } from '../../lib/catalog';
import { useConnections } from '../../stores/connections';
import type { PaneId } from '../../stores/paneTypes';
import { usePanes } from '../../stores/panes';
import { useUi } from '../../stores/ui';
import controls from '../ui/controls.module.css';
import { Icon } from '../ui/Icon';
import { ColorDot } from './ColorDot';
import styles from './ConnectionPicker.module.css';
import { buildGroups, hiddenCount, sourceIcon } from './pickerGroups';

interface Props {
  paneId: PaneId;
  catalog: readonly KnownConnection[];
  /** The pane's connection, shown first when no section offers it. */
  current: KnownConnection | null;
  /** Viewport point under the trigger's left edge. */
  anchor: { x: number; y: number };
  trigger: RefObject<HTMLElement | null>;
  /** `refocus` returns focus to the trigger. */
  onClose: (refocus: boolean) => void;
}

interface Action {
  label: string;
  run: () => void;
}

const WIDTH = 300;
const MARGIN = 8;

/** Filter field, grouped connection list and actions. Focus stays in the field; arrows move the active row. */
export function ConnectionPickerPopover({
  paneId,
  catalog,
  current,
  anchor,
  trigger,
  onClose,
}: Props) {
  const sshUnavailable = useConnections((s) => s.sshUnavailable);
  const connId = usePanes((s) => s.panes[paneId].connId);
  const [query, setQuery] = useState('');
  const [showHidden, setShowHidden] = useState(false);
  // Null until an arrow key or the pointer picks a row; then the current or first match is active.
  const [picked, setPicked] = useState<number | null>(null);
  const root = useRef<HTMLDivElement>(null);
  const input = useRef<HTMLInputElement>(null);
  const uid = useId();

  const groups = buildGroups(catalog, { current, query, showHidden, sshUnavailable });
  const rows = groups.flatMap((g) => g.rows);
  const hidden = hiddenCount(catalog);

  const connect = (conn: KnownConnection) => {
    if (conn.id !== connId) void usePanes.getState().connectTo(paneId, conn.target, conn.name);
    onClose(true);
  };

  const actions: Action[] = [
    ...(hidden > 0
      ? [
          {
            label: `${hidden} hidden · ${showHidden ? 'Hide' : 'Show'}`,
            run: () => {
              setShowHidden(!showHidden);
              setPicked(null);
            },
          },
        ]
      : []),
    {
      label: 'Quick connect…',
      run: () => {
        onClose(true);
        useUi.getState().openDialog({ kind: 'quickConnect', paneId });
      },
    },
    {
      label: 'Manage connections…',
      run: () => {
        const known = connId !== null && catalog.some((c) => c.id === connId);
        onClose(true);
        useUi.getState().openDialog({ kind: 'connections', ...(known && { connId }) });
      },
    },
  ];

  const total = rows.length + actions.length;
  const currentAt = rows.findIndex((r) => r.conn.id === current?.id);
  const fallback = rows.length === 0 ? -1 : query.trim() === '' ? Math.max(currentAt, 0) : 0;
  const active = picked !== null && picked < total ? picked : fallback;

  const run = (i: number) => {
    const row = rows[i];
    if (row) connect(row.conn);
    else actions[i - rows.length]?.run();
  };

  useEffect(() => {
    if (active < 0) return;
    root.current?.querySelector('[data-active="true"]')?.scrollIntoView?.({ block: 'nearest' });
  }, [active]);

  useEffect(() => {
    const away = (event: PointerEvent) => {
      const target = event.target as Node;
      if (!root.current?.contains(target) && !trigger.current?.contains(target)) onClose(false);
    };
    const leave = () => onClose(false);
    document.addEventListener('pointerdown', away, true);
    window.addEventListener('blur', leave);
    window.addEventListener('resize', leave);
    return () => {
      document.removeEventListener('pointerdown', away, true);
      window.removeEventListener('blur', leave);
      window.removeEventListener('resize', leave);
    };
  }, [onClose, trigger]);

  const onKeyDown = (e: KeyboardEvent) => {
    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault();
        setPicked(active < 0 ? 0 : (active + 1) % total);
        break;
      case 'ArrowUp':
        e.preventDefault();
        setPicked(active < 0 ? total - 1 : (active - 1 + total) % total);
        break;
      case 'Home':
        e.preventDefault();
        setPicked(0);
        break;
      case 'End':
        e.preventDefault();
        setPicked(total - 1);
        break;
      case 'Enter':
        e.preventDefault();
        if (active >= 0) run(active);
        break;
      case 'Escape':
        e.preventDefault();
        e.stopPropagation();
        onClose(true);
        break;
      case 'Tab':
        // Focus lands on the trigger first, so the browser's Tab moves on from there.
        onClose(true);
        break;
      default:
        break;
    }
  };

  const width = Math.min(WIDTH, window.innerWidth - 2 * MARGIN);
  const style = {
    left: Math.max(MARGIN, Math.min(anchor.x, window.innerWidth - width - MARGIN)),
    top: anchor.y,
    width,
    maxHeight: window.innerHeight - anchor.y - MARGIN,
  };
  const optionId = (i: number) => `${uid}-${i}`;
  const listId = `${uid}-list`;

  return (
    <div
      ref={root}
      className={styles.popover}
      style={style}
      onKeyDown={onKeyDown}
      onMouseDown={(e) => {
        // Keep focus in the field when pressing headings, rows and buttons.
        if (e.target !== input.current) e.preventDefault();
      }}
    >
      <input
        ref={input}
        className={`${controls.field} ${styles.search}`}
        role="combobox"
        aria-label="Search"
        aria-expanded="true"
        aria-autocomplete="list"
        aria-controls={rows.length > 0 ? listId : undefined}
        aria-activedescendant={active >= 0 ? optionId(active) : undefined}
        placeholder="Connect to…"
        value={query}
        autoFocus
        spellCheck={false}
        autoCapitalize="off"
        autoCorrect="off"
        onChange={(e) => {
          setQuery(e.target.value);
          setPicked(null);
        }}
      />
      {rows.length > 0 ? (
        <div id={listId} role="listbox" aria-label="Connect to" className={styles.list}>
          {groups.map((g) => (
            <div
              key={g.id}
              role={g.label === null ? 'presentation' : 'group'}
              aria-labelledby={g.label === null ? undefined : `${uid}-${g.id}`}
            >
              {g.label !== null && (
                <div id={`${uid}-${g.id}`} className={styles.heading}>
                  {g.label}
                </div>
              )}
              {g.notice !== undefined && <div className={styles.notice}>{g.notice}</div>}
              {g.rows.map((r) => {
                const i = rows.indexOf(r);
                const selected = r.conn.id === current?.id;
                return (
                  <div
                    key={r.conn.id}
                    id={optionId(i)}
                    role="option"
                    aria-selected={selected}
                    className={styles.row}
                    data-active={i === active}
                    data-hidden={r.hidden}
                    title={r.hidden ? 'Hidden from the picker' : undefined}
                    onMouseMove={() => active !== i && setPicked(i)}
                    onClick={() => connect(r.conn)}
                  >
                    <Icon name={sourceIcon(r.conn.source)} className={styles.icon} />
                    <span className={styles.label}>{r.conn.name}</span>
                    {r.hidden && <Icon name="eyeOff" size={14} className={styles.icon} />}
                    {r.conn.prefs.color && <ColorDot color={r.conn.prefs.color} />}
                    <span className={styles.check}>
                      {selected && <Icon name="check" size={14} />}
                    </span>
                  </div>
                );
              })}
            </div>
          ))}
        </div>
      ) : (
        <div className={styles.empty}>No matching connections</div>
      )}
      <div className={styles.actions}>
        {actions.map((a, j) => {
          const i = rows.length + j;
          return (
            <button
              key={a.label}
              id={optionId(i)}
              type="button"
              tabIndex={-1}
              className={styles.action}
              data-active={i === active}
              onMouseMove={() => active !== i && setPicked(i)}
              onClick={a.run}
            >
              {a.label}
            </button>
          );
        })}
      </div>
    </div>
  );
}
