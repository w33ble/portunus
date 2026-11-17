import { useEffect, useLayoutEffect, useRef, useState } from 'react';
import styles from './ContextMenu.module.css';

export interface MenuItem {
  label: string;
  shortcut?: string;
  disabled?: boolean;
  onSelect: () => void;
  /** Draws a divider above this item. */
  separator?: boolean;
  /** Starts a titled section: a divider (unless first) and a heading above this item. */
  heading?: string;
  /** Secondary text after the label. */
  detail?: string;
  /** Explains a disabled item, shown under its label. */
  hint?: string;
}

interface Props {
  x: number;
  y: number;
  items: readonly MenuItem[];
  onClose: () => void;
}

/** Positions inside the viewport; closes on outside press, Escape, blur and resize. */
export function ContextMenu({ x, y, items, onClose }: Props) {
  const ref = useRef<HTMLDivElement>(null);
  const [pos, setPos] = useState({ left: x, top: y });
  const [focused, setFocused] = useState(() => items.findIndex((i) => !i.disabled));

  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const { width, height } = el.getBoundingClientRect();
    setPos({
      left: Math.max(4, Math.min(x, window.innerWidth - width - 4)),
      top: Math.max(4, Math.min(y, window.innerHeight - height - 4)),
    });
    el.focus();
  }, [x, y]);

  useEffect(() => {
    const away = (event: PointerEvent) => {
      if (!ref.current?.contains(event.target as Node)) onClose();
    };
    document.addEventListener('pointerdown', away, true);
    window.addEventListener('blur', onClose);
    window.addEventListener('resize', onClose);
    return () => {
      document.removeEventListener('pointerdown', away, true);
      window.removeEventListener('blur', onClose);
      window.removeEventListener('resize', onClose);
    };
  }, [onClose]);

  const step = (dir: 1 | -1) => {
    for (let i = 1; i <= items.length; i++) {
      const next = (focused + dir * i + items.length * 2) % items.length;
      if (!items[next]?.disabled) {
        setFocused(next);
        return;
      }
    }
  };

  const choose = (item: MenuItem | undefined) => {
    if (!item || item.disabled) return;
    onClose();
    item.onSelect();
  };

  return (
    <div
      ref={ref}
      role="menu"
      tabIndex={-1}
      className={styles.menu}
      style={pos}
      onContextMenu={(e) => e.preventDefault()}
      onKeyDown={(e) => {
        switch (e.key) {
          case 'ArrowDown':
            e.preventDefault();
            step(1);
            break;
          case 'ArrowUp':
            e.preventDefault();
            step(-1);
            break;
          case 'Enter':
            e.preventDefault();
            choose(items[focused]);
            break;
          case 'Escape':
            e.preventDefault();
            e.stopPropagation();
            onClose();
            break;
          default:
            break;
        }
      }}
    >
      {items.map((item, i) => (
        <div key={i}>
          {(item.separator || (item.heading !== undefined && i > 0)) && (
            <div role="separator" className={styles.separator} />
          )}
          {item.heading !== undefined && (
            <div role="presentation" className={styles.heading}>
              {item.heading}
            </div>
          )}
          <button
            type="button"
            role="menuitem"
            tabIndex={-1}
            className={styles.item}
            data-focused={i === focused}
            disabled={item.disabled}
            title={item.hint}
            onMouseEnter={() => !item.disabled && setFocused(i)}
            onClick={() => choose(item)}
          >
            <span className={styles.text}>
              <span className={styles.label}>{item.label}</span>
              {item.hint && <span className={styles.hint}>{item.hint}</span>}
            </span>
            {item.detail && <span className={styles.detail}>{item.detail}</span>}
            {item.shortcut && <span className={styles.shortcut}>{item.shortcut}</span>}
          </button>
        </div>
      ))}
    </div>
  );
}
