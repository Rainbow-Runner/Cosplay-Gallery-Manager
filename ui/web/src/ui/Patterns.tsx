import { useEffect, useRef, type ReactNode } from "react";

import { Icon } from "./Icon";
import { IconButton } from "./Primitives";

function classes(...values: Array<string | false | undefined>) {
  return values.filter(Boolean).join(" ");
}

export function Breadcrumbs({ children, label = "Breadcrumb" }: { children: ReactNode; label?: string }) {
  return <nav className="cgm-breadcrumbs" aria-label={label}><ol>{children}</ol></nav>;
}

interface PaginationProps {
  page: number;
  totalPages: number;
  previousLabel: string;
  nextLabel: string;
  onPageChange: (page: number) => void;
}

export function Pagination({ page, totalPages, previousLabel, nextLabel, onPageChange }: PaginationProps) {
  return (
    <nav className="cgm-pagination" aria-label="Pagination">
      <button type="button" disabled={page <= 1} onClick={() => onPageChange(page - 1)}>{previousLabel}</button>
      <span aria-current="page">{page} / {Math.max(totalPages, 1)}</span>
      <button type="button" disabled={page >= totalPages} onClick={() => onPageChange(page + 1)}>{nextLabel}</button>
    </nav>
  );
}

interface Tab {
  id: string;
  label: string;
}

export function Tabs({ items, active, onChange, label }: { items: Tab[]; active: string; onChange: (id: string) => void; label: string }) {
  return (
    <div className="cgm-tabs" role="tablist" aria-label={label}>
      {items.map((item) => (
        <button key={item.id} type="button" role="tab" aria-selected={item.id === active} onClick={() => onChange(item.id)}>
          {item.label}
        </button>
      ))}
    </div>
  );
}

interface DrawerProps {
  open: boolean;
  title: string;
  closeLabel: string;
  onClose: () => void;
  children: ReactNode;
}

export function Drawer({ open, title, closeLabel, onClose, children }: DrawerProps) {
  const panelRef = useRef<HTMLElement>(null);
  const closeRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!open) return;
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    closeRef.current?.focus();
    return () => { document.body.style.overflow = previousOverflow; };
  }, [open]);

  if (!open) return null;

  function keyDown(event: React.KeyboardEvent<HTMLElement>) {
    if (event.key === "Escape") {
      event.preventDefault();
      onClose();
      return;
    }
    if (event.key !== "Tab") return;
    const focusable = Array.from(panelRef.current?.querySelectorAll<HTMLElement>(
      'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
    ) ?? []);
    if (!focusable.length) return;
    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first.focus();
    }
  }

  return (
    <div className="cgm-drawer-layer" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}>
      <aside ref={panelRef} className="cgm-drawer" role="dialog" aria-modal="true" aria-label={title} onKeyDown={keyDown}>
        <header>
          <strong>{title}</strong>
          <IconButton ref={closeRef} variant="ghost" label={closeLabel} onClick={onClose}><Icon name="close" /></IconButton>
        </header>
        {children}
      </aside>
    </div>
  );
}

interface DialogProps {
  titleID: string;
  onClose: () => void;
  children: ReactNode;
  className?: string;
  dismissible?: boolean;
}

export function Dialog({ titleID, onClose, children, className = "", dismissible = true }: DialogProps) {
  const panelRef = useRef<HTMLElement>(null);
  const previousFocusRef = useRef(document.activeElement instanceof HTMLElement ? document.activeElement : null);

  useEffect(() => {
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    const initial = panelRef.current?.querySelector<HTMLElement>("[autofocus], input, button, select, textarea, a[href]");
    initial?.focus();
    return () => {
      document.body.style.overflow = previousOverflow;
      previousFocusRef.current?.focus();
    };
  }, []);

  function keyDown(event: React.KeyboardEvent<HTMLElement>) {
    if (event.key === "Escape" && dismissible) {
      event.preventDefault();
      onClose();
      return;
    }
    if (event.key !== "Tab") return;
    const focusable = Array.from(panelRef.current?.querySelectorAll<HTMLElement>(
      'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
    ) ?? []);
    if (!focusable.length) return;
    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first.focus();
    }
  }

  return (
    <div className="operation-confirm-backdrop" role="presentation" onMouseDown={(event) => {
      if (dismissible && event.target === event.currentTarget) onClose();
    }}>
      <section ref={panelRef} className={classes("operation-confirm", className)} role="dialog" aria-modal="true" aria-labelledby={titleID} onKeyDown={keyDown}>
        {children}
      </section>
    </div>
  );
}

export function DataTable({ children, label, className }: { children: ReactNode; label: string; className?: string }) {
  return <div className={classes("cgm-data-table", className)}><table aria-label={label}>{children}</table></div>;
}

export function EmptyState({ title, children }: { title: string; children?: ReactNode }) {
  return <section className="cgm-empty-state"><Icon name="images" /><h2>{title}</h2>{children}</section>;
}

export function ErrorState({ title, children }: { title: string; children?: ReactNode }) {
  return <section className="cgm-error-state" role="alert"><h2>{title}</h2>{children}</section>;
}
