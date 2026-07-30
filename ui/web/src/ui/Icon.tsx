import type { ReactNode, SVGProps } from "react";

export type IconName =
  | "book"
  | "calendar"
  | "character"
  | "chevron-left"
  | "chevron-right"
  | "close"
  | "gallery"
  | "heart"
  | "history"
  | "home"
  | "images"
  | "info"
  | "menu"
  | "search"
  | "settings"
  | "shuffle"
  | "sparkles"
  | "tags"
  | "user";

interface IconProps extends Omit<SVGProps<SVGSVGElement>, "children"> {
  name: IconName;
  label?: string;
}

const paths: Record<IconName, ReactNode> = {
  book: (
    <>
      <path d="M4 5.5A2.5 2.5 0 0 1 6.5 3H11v16H6.5A2.5 2.5 0 0 0 4 21.5Z" />
      <path d="M20 5.5A2.5 2.5 0 0 0 17.5 3H13v16h4.5a2.5 2.5 0 0 1 2.5 2.5Z" />
    </>
  ),
  calendar: (
    <>
      <rect x="3" y="5" width="18" height="16" rx="2" />
      <path d="M8 3v4M16 3v4M3 10h18" />
    </>
  ),
  character: (
    <>
      <circle cx="12" cy="8" r="3" />
      <path d="M6 21v-2a6 6 0 0 1 12 0v2M4 4h2M18 4h2M4 8h1M19 8h1" />
    </>
  ),
  "chevron-left": <path d="m15 18-6-6 6-6" />,
  "chevron-right": <path d="m9 18 6-6-6-6" />,
  close: <path d="m5 5 14 14M19 5 5 19" />,
  gallery: (
    <>
      <rect x="3.5" y="4.5" width="17" height="15" rx="2" />
      <path d="m6.5 16 3.6-4 2.7 2.7 1.9-2 2.8 3.3" />
      <circle cx="15.8" cy="9.1" r="1.35" />
    </>
  ),
  heart: <path d="M20.2 5.8a5.1 5.1 0 0 0-7.2 0L12 6.9l-1-1.1a5.1 5.1 0 0 0-7.2 7.2l1 1L12 21l7.2-7a5.1 5.1 0 0 0 1-8.2Z" />,
  history: (
    <>
      <path d="M3 12a9 9 0 1 0 3-6.7L3 8" />
      <path d="M3 3v5h5M12 7v5l3 2" />
    </>
  ),
  home: (
    <>
      <path d="m3 11 9-8 9 8" />
      <path d="M5.5 9.5V21h13V9.5M9.5 21v-6h5v6" />
    </>
  ),
  images: (
    <>
      <rect x="5" y="3" width="16" height="16" rx="2" />
      <path d="m8 15 3.5-4 2.5 2.5 1.5-1.5 2.5 3" />
      <path d="M3 7v12a2 2 0 0 0 2 2h12" />
    </>
  ),
  info: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 10.5v6" />
      <path d="M12 7.3h.01" />
    </>
  ),
  search: (
    <>
      <circle cx="10.8" cy="10.8" r="6.8" />
      <path d="m16 16 4.2 4.2" />
    </>
  ),
  settings: (
    <>
      <circle cx="12" cy="12" r="3" />
      <path d="M19.2 13.4a7.8 7.8 0 0 0 0-2.8l2-1.5-2-3.4-2.4 1a8 8 0 0 0-2.4-1.4L14 2.8h-4l-.4 2.5a8 8 0 0 0-2.4 1.4l-2.4-1-2 3.4 2 1.5a7.8 7.8 0 0 0 0 2.8l-2 1.5 2 3.4 2.4-1a8 8 0 0 0 2.4 1.4l.4 2.5h4l.4-2.5a8 8 0 0 0 2.4-1.4l2.4 1 2-3.4-2-1.5Z" />
    </>
  ),
  menu: <path d="M4 7h16M4 12h16M4 17h16" />,
  shuffle: (
    <>
      <path d="M3 7h3c4 0 6 10 10 10h5" />
      <path d="m18 14 3 3-3 3M3 17h3c1.7 0 3-1.8 4.2-4M14 7.8A4.3 4.3 0 0 1 16 7h5" />
      <path d="m18 4 3 3-3 3" />
    </>
  ),
  sparkles: (
    <>
      <path d="m12 3 1.2 3.8L17 8l-3.8 1.2L12 13l-1.2-3.8L7 8l3.8-1.2Z" />
      <path d="m5 14 .8 2.2L8 17l-2.2.8L5 20l-.8-2.2L2 17l2.2-.8ZM19 13l.7 1.8 1.8.7-1.8.7L19 18l-.7-1.8-1.8-.7 1.8-.7Z" />
    </>
  ),
  tags: (
    <>
      <path d="M20 13 13 20l-9-9V4h7Z" />
      <circle cx="8.2" cy="8.2" r="1.2" />
      <path d="m14 5 6 6" />
    </>
  ),
  user: (
    <>
      <circle cx="12" cy="8" r="4" />
      <path d="M4.5 21a7.5 7.5 0 0 1 15 0" />
    </>
  ),
};

export function Icon({ name, label, className = "", ...props }: IconProps) {
  return (
    <svg
      {...props}
      className={`cgm-icon ${className}`.trim()}
      viewBox="0 0 24 24"
      fill="none"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden={label ? undefined : true}
      aria-label={label}
      role={label ? "img" : undefined}
    >
      {paths[name]}
    </svg>
  );
}
