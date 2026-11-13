import type { ReactElement, SVGProps } from 'react';

const PATHS = {
  folder:
    'M1.75 4.25a1 1 0 0 1 1-1h3.1l1.4 1.5h5a1 1 0 0 1 1 1v6a1 1 0 0 1-1 1h-9.5a1 1 0 0 1-1-1z',
  file: 'M3.75 1.75h5l3.5 3.5v8a1 1 0 0 1-1 1h-7.5a1 1 0 0 1-1-1v-10.5a1 1 0 0 1 1-1zM8.75 1.75v3.5h3.5',
  link: 'M6.5 9.5l3-3M7 4.5l1-1a2.5 2.5 0 0 1 3.5 3.5l-1 1M9 11.5l-1 1A2.5 2.5 0 0 1 4.5 9l1-1',
  up: 'M8 13V3.5M4 7l4-4 4 4',
  refresh: 'M13 8a5 5 0 1 1-1.6-3.7M13 2.5v3h-3',
  newFolder:
    'M1.75 4.25a1 1 0 0 1 1-1h3.1l1.4 1.5h5a1 1 0 0 1 1 1v1.5M1.75 4.25v7.5a1 1 0 0 0 1 1h5.5M12 9.5v4M10 11.5h4',
  chevronRight: 'M6 3.5L10.5 8 6 12.5',
  chevronDown: 'M3.5 6L8 10.5 12.5 6',
  sortUp: 'M4 10l4-4 4 4',
  sortDown: 'M4 6l4 4 4-4',
  gear: 'M8 5.5a2.5 2.5 0 1 0 0 5 2.5 2.5 0 0 0 0-5zM8 1.5v1.6M8 12.9v1.6M1.5 8h1.6M12.9 8h1.6M3.4 3.4l1.1 1.1M11.5 11.5l1.1 1.1M3.4 12.6l1.1-1.1M11.5 4.5l1.1-1.1',
  close: 'M4 4l8 8M12 4l-8 8',
  warning: 'M8 2.5l6 10.5H2zM8 6.5v3M8 11.2v.1',
  retry: 'M13 8a5 5 0 1 1-1.6-3.7M13 2.5v3h-3',
  arrowRight: 'M2.5 8h11M10 4.5L13.5 8 10 11.5',
  copy: 'M5.5 5.5v-2a1 1 0 0 1 1-1h6a1 1 0 0 1 1 1v6a1 1 0 0 1-1 1h-2M2.5 6.5a1 1 0 0 1 1-1h6a1 1 0 0 1 1 1v6a1 1 0 0 1-1 1h-6a1 1 0 0 1-1-1z',
  trash:
    'M2.5 4.5h11M6 4.5v-1a1 1 0 0 1 1-1h2a1 1 0 0 1 1 1v1M4 4.5l.6 8a1 1 0 0 0 1 .9h4.8a1 1 0 0 0 1-.9l.6-8',
  edit: 'M10.5 2.5l3 3-8 8H2.5v-3z',
  cloud: 'M4.5 12.5a3 3 0 0 1-.3-6 4 4 0 0 1 7.6.8 2.6 2.6 0 0 1-.3 5.2z',
  server: 'M2.5 3.5h11v3.5h-11zM2.5 9h11v3.5h-11zM4.5 5.25h.01M4.5 10.75h.01',
  check: 'M3.5 8.5l3 3 6-7',
} as const;

export type IconName = keyof typeof PATHS;

interface IconProps extends Omit<SVGProps<SVGSVGElement>, 'name'> {
  name: IconName;
  size?: number;
}

export function Icon({ name, size = 16, ...rest }: IconProps): ReactElement {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.5"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      {...rest}
    >
      <path d={PATHS[name]} />
    </svg>
  );
}
