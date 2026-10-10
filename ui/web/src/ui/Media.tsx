import type { ReactNode } from "react";

function initials(name: string) {
  const segments = name.trim().split(/\s+/u).filter(Boolean);
  return (segments.length > 1 ? `${segments[0][0]}${segments.at(-1)?.[0] ?? ""}` : name.slice(0, 2)).toUpperCase();
}

export function Avatar({ name, src, size = "medium" }: { name: string; src?: string | null; size?: "small" | "medium" | "large" }) {
  return <span className={`cgm-avatar cgm-avatar--${size}`} title={name}>{src ? <img src={src} alt="" loading="lazy" /> : <span aria-hidden="true">{initials(name)}</span>}<span className="sr-only">{name}</span></span>;
}

export function CardMedia({ children, className = "" }: { children: ReactNode; className?: string }) {
  return <div className={`cgm-card-media ${className}`.trim()}>{children}</div>;
}

export function Skeleton({ className = "" }: { className?: string }) {
  return <span className={`cgm-skeleton ${className}`.trim()} aria-hidden="true" />;
}
