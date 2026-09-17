import { useEffect, useMemo, useState, type ReactNode } from "react";
import { useIntl } from "react-intl";

import type { ManageTagTreeItem } from "./types";

export function ManageTagHierarchy({ items, selected, query, onSelect, onCreateChild }: {
  items: ManageTagTreeItem[];
  selected: string;
  query: string;
  onSelect: (uuid: string) => void;
  onCreateChild: (item: ManageTagTreeItem) => void;
}) {
  const intl = useIntl();
  const f = (id: string, values?: Record<string, string | number>) => intl.formatMessage({ id: `manage.tagTree.${id}` }, values);
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [scope, setScope] = useState<"ALL" | "ROOT" | "UNCLASSIFIED">("ALL");
  const byID = useMemo(() => new Map(items.map((item) => [item.uuid, item])), [items]);
  const children = useMemo(() => {
    const value = new Map<string, ManageTagTreeItem[]>();
    for (const item of items) for (const parent of item.parentUUIDs) {
      const siblings = value.get(parent) || [];
      siblings.push(item);
      value.set(parent, siblings);
    }
    return value;
  }, [items]);
  const normalizedQuery = query.trim().toLocaleLowerCase();
  const visible = useMemo(() => {
    if (!normalizedQuery) return null;
    const matched = new Set<string>();
    const visit = (uuid: string) => {
      if (matched.has(uuid)) return;
      matched.add(uuid);
      for (const parent of byID.get(uuid)?.parentUUIDs || []) visit(parent);
    };
    for (const item of items) if ([item.name, ...item.aliases].some((name) => name.toLocaleLowerCase().includes(normalizedQuery))) visit(item.uuid);
    return matched;
  }, [byID, items, normalizedQuery]);
  useEffect(() => {
    if (!selected) return;
    setExpanded((previous) => {
      const next = new Set(previous);
      const visit = (uuid: string) => {
        for (const parent of byID.get(uuid)?.parentUUIDs || []) {
          if (next.has(parent)) continue;
          next.add(parent);
          visit(parent);
        }
      };
      visit(selected);
      return next;
    });
  }, [byID, selected]);
  const roots = items.filter((item) => item.parentUUIDs.length === 0);
  const unclassified = items.filter((item) => item.parentUUIDs.length === 0 && item.galleryCount === 0 && item.childCount === 0);
  const topLevel = scope === "UNCLASSIFIED" ? unclassified : roots;
  const render = (item: ManageTagTreeItem, path: Set<string>): ReactNode => {
    if (path.has(item.uuid) || (visible && !visible.has(item.uuid))) return null;
    const childItems = children.get(item.uuid) || [];
    const open = Boolean(normalizedQuery) || expanded.has(item.uuid);
    const nextPath = new Set(path).add(item.uuid);
    return <div className="manage-tag-tree__branch" key={`${Array.from(path).join(":")}:${item.uuid}`}>
      <div className={`manage-tag-tree__row${selected === item.uuid ? " is-active" : ""}`}>
        <button type="button" className="manage-tag-tree__expand" disabled={!childItems.length || scope === "ROOT"} aria-label={f(open ? "collapse" : "expand", { name: item.name })} aria-expanded={childItems.length && scope !== "ROOT" ? open : undefined} onClick={() => setExpanded((previous) => { const next = new Set(previous); if (next.has(item.uuid)) next.delete(item.uuid); else next.add(item.uuid); return next; })}>{childItems.length && scope !== "ROOT" ? open ? "▾" : "▸" : "·"}</button>
        <button type="button" className="manage-tag-tree__name" onClick={() => onSelect(item.uuid)} title={item.name}>{item.name}</button>
        <span className="manage-tag-tree__counts" title={f("counts")}>{item.childCount} / {item.galleryCount}</span>
        <button type="button" className="manage-tag-tree__add" aria-label={f("newChildOf", { name: item.name })} title={f("newChild") } onClick={() => onCreateChild(item)}>＋</button>
      </div>
      {open && scope !== "ROOT" && childItems.length ? <div className="manage-tag-tree__children">{childItems.map((child) => render(child, nextPath))}</div> : null}
    </div>;
  };
  return <div className="manage-tag-tree">
    <div className="manage-tag-tree__filters" role="group" aria-label={f("scope")}>
      {(["ALL", "ROOT", "UNCLASSIFIED"] as const).map((value) => <button type="button" key={value} className={scope === value ? "is-active" : ""} onClick={() => setScope(value)}>{f(value.toLowerCase())}</button>)}
    </div>
    {normalizedQuery ? <p className="manage-tag-tree__hint">{f("searchHint")}</p> : <p className="manage-tag-tree__hint">{f("hint")}</p>}
    <div className="manage-tag-tree__scroll">{topLevel.map((item) => render(item, new Set()))}</div>
    {visible && !visible.size ? <p className="entity-list-state">{f("noMatches")}</p> : null}
    {!visible && !topLevel.length ? <p className="entity-list-state">{f(scope === "UNCLASSIFIED" ? "noUnclassified" : "empty")}</p> : null}
  </div>;
}
