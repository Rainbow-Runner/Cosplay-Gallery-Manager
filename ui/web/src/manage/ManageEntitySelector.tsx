import { useLazyQuery } from "@apollo/client/react";
import { useEffect, useState } from "react";
import { MANAGE_CORE_ENTITY_OPTIONS } from "../api/manage";
import type { ManageCoreEntity } from "./types";

export function ManageEntitySelector({
  kind, label, uuid, name, onSelect,
}: {
  kind: ManageCoreEntity["kind"];
  label: string;
  uuid: string;
  name: string;
  onSelect: (entity: Pick<ManageCoreEntity, "uuid" | "name" | "workUUID" | "workName" | "metadataRevision">) => void;
}) {
  const [search, setSearch] = useState("");
  const [open, setOpen] = useState(false);
  const [load, state] = useLazyQuery<{ manageCoreEntityOptions: ManageCoreEntity[] }>(MANAGE_CORE_ENTITY_OPTIONS);
  useEffect(() => {
    if (!search.trim()) return;
    const timer = window.setTimeout(() => void load({ variables: { kind, query: search.trim(), limit: 20 } }), 180);
    return () => window.clearTimeout(timer);
  }, [kind, load, search]);
  const options = state.data?.manageCoreEntityOptions || [];
  return <div className="manage-entity-selector">
    <label>{label} UUID<input value={uuid} onChange={(event) => onSelect({ uuid: event.target.value, name: "", workUUID: "", workName: "", metadataRevision: 0 })} /></label>
    <span>{name || "No entity selected"}</span>
    <div className="manage-entity-selector__search"><input aria-label={`Search ${label}`} placeholder={`Search ${label} name or alias`} value={search} onFocus={() => { setOpen(true); if (!search) void load({ variables: { kind, query: "", limit: 20 } }); }} onChange={(event) => { setSearch(event.target.value); setOpen(true); }} />
      {open && state.called && (state.loading || options.length) ? <div className="manage-entity-selector__options" role="listbox">{state.loading ? <span>Searching…</span> : options.map((entity) => <button type="button" role="option" aria-selected={entity.uuid === uuid} key={entity.uuid} onClick={() => { onSelect(entity); setSearch(""); setOpen(false); }}><strong>{entity.name}</strong><small>{entity.kind === "CHARACTER" && entity.workName ? `${entity.workName} · ` : ""}{entity.aliases.join(" / ") || entity.uuid}</small></button>)}</div> : null}
    </div>
  </div>;
}
