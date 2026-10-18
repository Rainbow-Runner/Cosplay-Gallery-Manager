import { useState } from "react";
import { useIntl } from "react-intl";
import { Dialog } from "../ui/Patterns";
import { formatStorageBytes } from "./ManageSettingsPage";

export interface CacheCleanupReview {
  referenced_bytes: number; obsolete_bytes: number; pending_bytes: number;
  pending_files: number; failed_files: number; orphan_bytes: number;
  reclaimable_bytes: number; partial: boolean; ignored_files: number; grace_hours: number;
  candidates: { id: string; reason: "OBSOLETE" | "PENDING_DELETE" | "ORPHAN"; byte_size: number; last_error_code?: string }[];
}

const endpoint = "/manage/cache/review";

export function ManageCacheCleanupPanel() {
  const intl = useIntl();
  const f = (key: string, values?: Record<string, string | number>) => intl.formatMessage({ id: `manage.cacheCleanup.${key}` }, values);
  const [review, setReview] = useState<CacheCleanupReview | null>(null);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [busy, setBusy] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [message, setMessage] = useState("");

  async function preview() {
    setBusy(true); setMessage(""); setSelected(new Set()); setReview(null);
    try {
      const response = await fetch(endpoint, { credentials: "same-origin", cache: "no-store" });
      if (!response.ok) throw new Error("CACHE_REVIEW_FAILED");
      setReview(await response.json() as CacheCleanupReview);
    } catch { setMessage(f("reviewFailed")); }
    finally { setBusy(false); }
  }

  async function clean() {
    if (selected.size === 0 || selected.size > 100 || password === "" || confirmation !== "CLEAN") return;
    setBusy(true); setMessage("");
    try {
      const response = await fetch(endpoint, { method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ ids: [...selected], password, confirmation }) });
      if (!response.ok) { setMessage(f(response.status === 409 ? "stale" : "cleanFailed")); return; }
      const result = await response.json() as { removed: number; freed_bytes: number; failed: number; skipped: number };
      setMessage(f("result", { removed: result.removed, bytes: formatStorageBytes(result.freed_bytes), failed: result.failed, skipped: result.skipped }));
    } catch { setMessage(f("cleanFailed")); }
    finally {
      setBusy(false); setConfirming(false); setPassword(""); setConfirmation(""); setSelected(new Set()); setReview(null);
    }
  }

  const selectedBytes = review?.candidates.filter((candidate) => selected.has(candidate.id)).reduce((sum, candidate) => sum + candidate.byte_size, 0) ?? 0;
  return <section className="operation-panel cache-cleanup-panel">
    <header><div><h3>{f("title")}</h3><p>{f("description")}</p></div><button type="button" disabled={busy} onClick={() => void preview()}>{f(busy ? "working" : "preview")}</button></header>
    {message ? <p className="manage-message" role="status">{message}</p> : null}
    {review ? <>
      <dl className="cache-cleanup-summary">
        <div><dt>{f("valid")}</dt><dd>{formatStorageBytes(Math.max(0, review.referenced_bytes - review.obsolete_bytes))}</dd></div>
        <div><dt>{f("obsolete")}</dt><dd>{formatStorageBytes(review.obsolete_bytes)}</dd></div>
        <div><dt>{f("pending")}</dt><dd>{formatStorageBytes(review.pending_bytes)} · {review.pending_files}</dd></div>
        <div><dt>{f("orphan")}</dt><dd>{formatStorageBytes(review.orphan_bytes)}</dd></div>
        <div><dt>{f("reclaimable")}</dt><dd>{formatStorageBytes(review.reclaimable_bytes)}</dd></div>
      </dl>
      <p>{f("protected", { hours: review.grace_hours })}</p>
      {review.partial ? <p role="status">{f("partial")}</p> : null}
      <p>{f("ignored", { count: review.ignored_files, failed: review.failed_files })}</p>
      <div className="manage-table-wrap"><table className="manage-table"><thead><tr><th>{f("select")}</th><th>{f("reason")}</th><th>{f("size")}</th><th>{f("error")}</th></tr></thead><tbody>
        {review.candidates.map((candidate) => <tr key={candidate.id}><td><input type="checkbox" aria-label={f("selectItem", { id: candidate.id.slice(0, 8) })} checked={selected.has(candidate.id)} disabled={busy || (!selected.has(candidate.id) && selected.size >= 100)} onChange={() => setSelected((current) => { const next = new Set(current); if (next.has(candidate.id)) next.delete(candidate.id); else next.add(candidate.id); return next; })} /></td><td>{f(`reason.${candidate.reason}`)}<small>{candidate.id.slice(0, 12)}</small></td><td>{formatStorageBytes(candidate.byte_size)}</td><td>{candidate.last_error_code || "—"}</td></tr>)}
      </tbody></table></div>
      {review.candidates.length === 0 ? <p>{f("empty")}</p> : null}
      <footer><button type="button" disabled={busy || review.candidates.length === 0} onClick={() => setSelected(new Set(review.candidates.slice(0, 100).map((candidate) => candidate.id)))}>{f("selectBatch")}</button><button className="danger" type="button" disabled={busy || selected.size === 0} onClick={() => setConfirming(true)}>{f("cleanSelected", { count: selected.size, bytes: formatStorageBytes(selectedBytes) })}</button></footer>
    </> : null}
    {confirming ? <Dialog titleID="cache-cleanup-title" dismissible={!busy} onClose={() => { setConfirming(false); setPassword(""); setConfirmation(""); }}>
      <h3 id="cache-cleanup-title">{f("confirmTitle")}</h3><p>{f("confirmBody", { count: selected.size, bytes: formatStorageBytes(selectedBytes) })}</p>
      <label>{f("password")}<input type="password" autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} disabled={busy} /></label>
      <label>{f("confirmation")}<input value={confirmation} onChange={(event) => setConfirmation(event.target.value)} disabled={busy} /></label>
      <footer><button type="button" disabled={busy} onClick={() => { setConfirming(false); setPassword(""); setConfirmation(""); }}>{f("cancel")}</button><button className="danger" type="button" disabled={busy || !password || confirmation !== "CLEAN" || selected.size === 0} onClick={() => void clean()}>{f("execute")}</button></footer>
    </Dialog> : null}
  </section>;
}
