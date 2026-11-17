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
  chevronLeft: 'M10 3.5L5.5 8 10 12.5',
  chevronRight: 'M6 3.5L10.5 8 6 12.5',
  chevronDown: 'M3.5 6L8 10.5 12.5 6',
  sortUp: 'M4 10l4-4 4 4',
  sortDown: 'M4 6l4 4 4-4',
  gear: 'M6.79 3.15L6.97 1.48L9.03 1.48L9.21 3.15L10.58 3.71L11.88 2.66L13.34 4.12L12.29 5.42L12.85 6.79L14.52 6.97L14.52 9.03L12.85 9.21L12.29 10.58L13.34 11.88L11.88 13.34L10.58 12.29L9.21 12.85L9.03 14.52L6.97 14.52L6.79 12.85L5.42 12.29L4.12 13.34L2.66 11.88L3.71 10.58L3.15 9.21L1.48 9.03L1.48 6.97L3.15 6.79L3.71 5.42L2.66 4.12L4.12 2.66L5.42 3.71zM10.1 8a2.1 2.1 0 1 1-4.2 0a2.1 2.1 0 1 1 4.2 0z',
  close: 'M4 4l8 8M12 4l-8 8',
  search: 'M7 11.5a4.5 4.5 0 1 0 0-9 4.5 4.5 0 0 0 0 9zM10.25 10.25l3.25 3.25',
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
  home: 'M2.5 7.5L8 3l5.5 4.5M4 6.5v7h8v-7',
  plus: 'M8 3v10M3 8h10',
  minus: 'M3 8h10',
  eye: 'M1.5 8s2.5-4.5 6.5-4.5S14.5 8 14.5 8 12 12.5 8 12.5 1.5 8 1.5 8zM8 10a2 2 0 1 0 0-4 2 2 0 0 0 0 4z',
  eyeOff:
    'M1.5 8s2.5-4.5 6.5-4.5S14.5 8 14.5 8 12 12.5 8 12.5 1.5 8 1.5 8zM8 10a2 2 0 1 0 0-4 2 2 0 0 0 0 4zM2.5 2.5l11 11',
  bookmark: 'M4.5 2.5h7v11L8 10.5l-3.5 3z',
  star: 'M8 2l1.8 3.7 4 .6-2.9 2.8.7 4L8 11.2l-3.6 1.9.7-4-2.9-2.8 4-.6z',
  grip: 'M6 4h.01M10 4h.01M6 8h.01M10 8h.01M6 12h.01M10 12h.01',
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
