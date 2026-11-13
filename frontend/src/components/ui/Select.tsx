import type { ReactElement, SelectHTMLAttributes } from 'react';
import controls from './controls.module.css';
import { Icon } from './Icon';

interface SelectProps extends SelectHTMLAttributes<HTMLSelectElement> {
  /** Class for the wrapper, which owns layout (width, flex). */
  className?: string;
}

/** Native <select> with flat styling; the popup menu stays native for keyboard and a11y. */
export function Select({ className, children, ...rest }: SelectProps): ReactElement {
  return (
    <span className={`${controls.selectWrap} ${className ?? ''}`}>
      <select className={controls.select} {...rest}>
        {children}
      </select>
      <Icon name="chevronDown" size={12} className={controls.selectIcon} />
    </span>
  );
}
