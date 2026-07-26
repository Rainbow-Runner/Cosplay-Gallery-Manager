import { useCallback, useEffect, useState } from "react";

import type { ManageCoserAssetCleanupResult, ManageCoserAssetReview } from "./types";

const reviewPath = "/manage/coser-assets/review";
const maximumSelection = 100;

export function ManageCoserAssetReviewPanel() {
  const [review, setReview] = useState<ManageCoserAssetReview | null>(null);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [loading, setLoading] = useState(true);
  const [cleaning, setCleaning] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [message, setMessage] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    setMessage("");
    setSelected(new Set());
    try {
      const response = await fetch(reviewPath, { credentials: "same-origin", cache: "no-store" });
      if (!response.ok) throw new Error(await response.text() || "Managed asset review failed.");
      setReview(await response.json() as ManageCoserAssetReview);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Managed asset review failed.");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  function toggle(id: string) {
    setSelected((current) => {
      const next = new Set(current);
      if (next.has(id)) next.delete(id);
      else if (next.size < maximumSelection) next.add(id);
      return next;
    });
  }

  async function cleanup() {
    if (selected.size === 0 || confirmation !== "CLEAN" || password === "") return;
    setCleaning(true);
    setMessage("");
    try {
      const response = await fetch(reviewPath, {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ asset_ids: [...selected], password, confirmation }),
      });
      if (!response.ok) throw new Error(await response.text() || "Managed asset cleanup failed.");
      const result = await response.json() as ManageCoserAssetCleanupResult;
      setReview(result.review);
      setSelected(new Set());
      setConfirming(false);
      setPassword("");
      setConfirmation("");
      setMessage(`Removed ${result.deleted_file_count} generated files (${formatBytes(result.deleted_byte_size)}).`);
    } catch (error) {
      const failure = error instanceof Error ? error.message : "Managed asset cleanup failed.";
      await load();
      setConfirming(false);
      setPassword("");
      setConfirmation("");
      setMessage(`${failure.trim()} Review refreshed; no automatic retry was attempted.`);
    } finally {
      setCleaning(false);
    }
  }

  const selectedGroups = review?.groups.filter((group) => selected.has(group.id)) ?? [];
  const selectedFiles = selectedGroups.reduce((total, group) => total + group.file_count, 0);
  const selectedBytes = selectedGroups.reduce((total, group) => total + group.byte_size, 0);

  return <section className="operation-panel coser-asset-review">
    <header><div><h3>Unreferenced Coser assets</h3><p>Review application-generated avatar and banner groups that are no longer referenced. Unknown files, links, Coser Manifests and all user media are excluded from cleanup.</p></div>
      <div className="coser-asset-review__actions"><button type="button" disabled={loading} onClick={() => void load()}>{loading ? "Reviewing…" : "Refresh review"}</button>
        <button className="danger" type="button" disabled={selected.size === 0 || cleaning} onClick={() => { setPassword(""); setConfirmation(""); setConfirming(true); }}>Clean selected…</button></div></header>
    {message ? <p className="manage-message" role="status">{message}</p> : null}
    <div className="coser-asset-review__summary"><span>{review?.groups.length ?? 0} groups</span><span>{review?.total_file_count ?? 0} files</span><span>{formatBytes(review?.total_byte_size ?? 0)}</span><span>{review?.ignored_entry_count ?? 0} unknown or unsafe entries ignored</span></div>
    <div className="manage-table-wrap"><table className="manage-table coser-asset-table"><thead><tr><th>Select</th><th>Reason / kind</th><th>Technical Coser</th><th>Generated files</th><th>Last changed</th></tr></thead><tbody>
      {review?.groups.map((group) => <tr key={group.id}><td><input aria-label={`Select asset group ${group.id}`} type="checkbox" checked={selected.has(group.id)} disabled={!selected.has(group.id) && selected.size >= maximumSelection} onChange={() => toggle(group.id)} /></td>
        <td><strong>{reasonLabel(group.reason)}</strong><small>{group.kind}</small></td><td><code>{group.coser_uuid}</code></td><td>{group.file_count}<small>{formatBytes(group.byte_size)}</small></td><td>{new Date(group.modified_at).toLocaleString()}</td></tr>)}
    </tbody></table></div>
    {!loading && review?.groups.length === 0 ? <p className="state-message">No unreferenced generated Coser assets require review.</p> : null}

    {confirming ? <div className="operation-confirm-backdrop" role="presentation"><section className="operation-confirm" role="dialog" aria-modal="true" aria-labelledby="coser-cleanup-title">
      <p>IRREVERSIBLE MANAGED FILE CLEANUP</p><h3 id="coser-cleanup-title">Remove {selectedFiles} generated files?</h3>
      <ul><li>{selected.size} selected avatar/banner groups use {formatBytes(selectedBytes)}.</li><li>The server rechecks every database reference immediately before cleanup.</li><li>Coser Manifests, unknown files and user media are never included.</li><li>Removed files can be recovered only from an earlier full backup.</li></ul>
      <label>Owner password<input type="password" autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} /></label>
      <label>Type <strong>CLEAN</strong> to continue<input value={confirmation} onChange={(event) => setConfirmation(event.target.value)} /></label>
      <footer><button type="button" disabled={cleaning} onClick={() => setConfirming(false)}>Cancel</button><button className="danger" type="button" disabled={password === "" || confirmation !== "CLEAN" || cleaning} onClick={() => void cleanup()}>{cleaning ? "Rechecking and cleaning…" : "Permanently remove selected files"}</button></footer>
    </section></div> : null}
  </section>;
}

function reasonLabel(reason: "REPLACED" | "MERGED_COSER" | "DELETED_COSER") {
  switch (reason) {
    case "MERGED_COSER": return "Merged Coser";
    case "DELETED_COSER": return "Deleted Coser";
    default: return "Replaced or unpublished";
  }
}

function formatBytes(value: number) {
  if (value < 1024) return `${value} B`;
  const units = ["KiB", "MiB", "GiB", "TiB"];
  let size = value / 1024;
  let unit = units[0];
  for (let index = 1; index < units.length && size >= 1024; index += 1) {
    size /= 1024;
    unit = units[index];
  }
  return `${size.toFixed(size >= 10 ? 1 : 2)} ${unit}`;
}
