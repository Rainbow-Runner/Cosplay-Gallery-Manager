import type { Scope } from "./types";

export function ScopeSelector({ value, onChange }: { value: Scope; onChange: (scope: Scope) => void }) {
  return <div className="scope-selector" role="group" aria-label="Scope">{(["LIST", "MAGIC", "ALL"] as Scope[]).map((scope) =>
    <button key={scope} type="button" className={scope === value ? "is-active" : ""} onClick={() => onChange(scope)}>{scope}</button>)}</div>;
}
