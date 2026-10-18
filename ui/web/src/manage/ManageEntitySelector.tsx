import { useLazyQuery } from "@apollo/client/react";
import { useEffect, useRef, useState } from "react";
import { MANAGE_CORE_ENTITY_OPTIONS } from "../api/manage";
import type { ManageCoreEntity } from "./types";

export function ManageEntitySelector({
  kind, label, uuid, name, onSelect, assignableOnly = false,
}: {
  kind: ManageCoreEntity["kind"];
  label: string;
  uuid: string;
  name: string;
  assignableOnly?: boolean;
  onSelect: (entity: Pick<ManageCoreEntity, "uuid" | "name" | "workUUID" | "workName" | "metadataRevision">) => void;
}) {
  const [search, setSearch] = useState("");
  const [open, setOpen] = useState(false);
  const searchRoot = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const dismissOutside = (event: PointerEvent) => {
      if (event.target instanceof Node && !searchRoot.current?.contains(event.target)) setOpen(false);
    };
    document.addEventListener("pointerdown", dismissOutside, true);
    return () => document.removeEventListener("pointerdown", dismissOutside, true);
  }, [open]);
  const [load, state] = useLazyQuery<{ manageCoreEntityOptions: ManageCoreEntity[] }>(MANAGE_CORE_ENTITY_OPTIONS);
  useEffect(() => {
    if (!search.trim()) return;
    const timer = window.setTimeout(() => void load({ variables: { kind, query: search.trim(), limit: 20, ...(assignableOnly ? { assignableOnly: true } : {}) } }), 180);
    return () => window.clearTimeout(timer);
  }, [assignableOnly, kind, load, search]);
  const options = state.data?.manageCoreEntityOptions || [];
  return <div className="manage-entity-selector">
    <label>{label} UUID<input value={uuid} onChange={(event) => onSelect({ uuid: event.target.value, name: "", workUUID: "", workName: "", metadataRevision: 0 })} /></label>
    <span>{name || "No entity selected"}</span>
    <div className="manage-entity-selector__search" ref={searchRoot}
      onBlur={(event) => { if (!event.currentTarget.contains(event.relatedTarget)) setOpen(false); }}
      onKeyDown={(event) => { if (event.key === "Escape") { setOpen(false); event.stopPropagation(); } }}>
      <input aria-label={`Search ${label}`} aria-expanded={open} placeholder={`Search ${label} name or alias`} value={search}
        onFocus={() => { setOpen(true); if (!search) void load({ variables: { kind, query: "", limit: 20, ...(assignableOnly ? { assignableOnly: true } : {}) } }); }}
        onClick={() => setOpen(true)}
        onChange={(event) => { setSearch(event.target.value); setOpen(true); }} />
      {open && state.called && (state.loading || options.length) ? <div className="manage-entity-selector__options" role="listbox">{state.loading ? <span>Searching…</span> : options.map((entity) => <button type="button" role="option" aria-selected={entity.uuid === uuid} key={entity.uuid} onClick={() => { onSelect(entity); setSearch(""); setOpen(false); }}><strong>{entity.name}</strong><small>{entity.kind === "CHARACTER" && entity.workName ? `${entity.workName} · ` : ""}{entity.aliases.join(" / ") || entity.uuid}</small></button>)}</div> : null}
    </div>
  </div>;
}
