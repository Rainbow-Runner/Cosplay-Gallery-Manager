import { useMutation, useQuery } from "@apollo/client/react";
import { useState } from "react";
import { useSearchParams } from "react-router-dom";

import { CREATE_FULL_BACKUP, MANAGE_OPERATIONS, RESTORE_BACKUP } from "../api/manage";
import type { ManageAuditPage, ManageBackupRecord, ManageMaintenanceState } from "./types";

interface OperationsData {
  manageBackups: ManageBackupRecord[];
  manageMaintenance: ManageMaintenanceState;
  manageAudit: ManageAuditPage;
}

export function ManageOperationsPage() {
  const [parameters, setParameters] = useSearchParams();
  const page = Math.max(1, Number(parameters.get("page")) || 1);
  const query = useQuery<OperationsData>(MANAGE_OPERATIONS, { variables: { page } });
  const [createBackup, createState] = useMutation<{ createFullBackup: ManageBackupRecord }>(CREATE_FULL_BACKUP);
  const [restoreBackup, restoreState] = useMutation<{ restoreBackup: ManageMaintenanceState }>(RESTORE_BACKUP);
  const [restoreTarget, setRestoreTarget] = useState<ManageBackupRecord | null>(null);
  const [confirmation, setConfirmation] = useState("");
  const [message, setMessage] = useState("");
  const data = query.data;

  async function create() {
    setMessage("");
    try {
      const result = await createBackup();
      if (result.data) setMessage(`Full backup ready: ${result.data.createFullBackup.fileName}`);
      await query.refetch();
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Backup creation failed.");
    }
  }

  async function restore() {
    if (!restoreTarget || confirmation !== "RESTORE") return;
    setMessage("");
    try {
      await restoreBackup({ variables: { backupID: restoreTarget.id } });
      window.location.assign("/login?restored=1");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Restore failed; the original installation was retained.");
      setRestoreTarget(null);
      setConfirmation("");
      await query.refetch();
    }
  }

  return <main className="manage-page operations-page">
    <header className="manage-heading"><div><p>BACKUP · RESTORE · AUDIT</p><h2>Operations</h2></div><button type="button" disabled={createState.loading || data?.manageMaintenance.state !== "NORMAL"} onClick={create}>{createState.loading ? "Creating consistent package…" : "Create full backup"}</button></header>
    {message ? <p className="manage-message" role="status">{message}</p> : null}
    {query.error ? <p className="manage-message">Unable to load operations state.</p> : null}
    <section className="operation-status"><div><span>Maintenance</span><strong>{data?.manageMaintenance.state ?? "…"}</strong></div><div><span>Backups</span><strong>{data?.manageBackups.length ?? 0}</strong></div><div><span>Audit events</span><strong>{data?.manageAudit.totalItems ?? 0}</strong></div></section>

    <section className="operation-panel"><header><div><h3>Application backups</h3><p>Full packages contain the consistent SQLite database, managed Coser metadata and necessary startup configuration. Media, caches and logs are excluded.</p></div></header>
      <div className="manage-table-wrap"><table className="manage-table backup-table"><thead><tr><th>Status</th><th>Kind / file</th><th>Size</th><th>Versions</th><th>Created</th><th>Action</th></tr></thead><tbody>
        {data?.manageBackups.map((backup) => <tr key={backup.id}><td><span className={`state state--${backup.status.toLowerCase()}`}>{backup.status}</span></td><td><strong>{backup.kind}</strong><small>{backup.fileName}</small><small>{backup.id}</small><small title={backup.archiveSHA256}>SHA-256 {backup.archiveSHA256 ? `${backup.archiveSHA256.slice(0, 16)}…` : "pending"}</small></td><td>{formatBytes(backup.byteSize)}</td><td><small>DB {backup.databaseSchemaVersion} · Manifest {backup.manifestSchemaVersion} · Media {backup.mediaProcessingVersion}</small><small>{backup.productVersion || "development"}</small></td><td>{new Date(backup.createdAt).toLocaleString()}</td><td><button type="button" disabled={backup.status !== "READY" || restoreState.loading || data.manageMaintenance.state !== "NORMAL"} onClick={() => { setRestoreTarget(backup); setConfirmation(""); }}>Restore…</button></td></tr>)}
      </tbody></table></div>
      {data && data.manageBackups.length === 0 ? <p className="state-message">No backups have been created.</p> : null}
    </section>

    <section className="operation-panel"><header><div><h3>Management audit</h3><p>Only high-impact operations and task summaries are recorded. Browsing, search, favourites and ratings are intentionally absent.</p></div></header>
      <div className="manage-table-wrap"><table className="manage-table audit-table"><thead><tr><th>Outcome</th><th>Event</th><th>Technical target</th><th>Summary</th><th>Time</th></tr></thead><tbody>
        {data?.manageAudit.items.map((event) => <tr key={event.id}><td><span className={`state state--${event.outcome.toLowerCase()}`}>{event.outcome}</span></td><td>{event.eventCode}<small>{event.errorCode || "—"}</small></td><td>{event.targetKind || "SYSTEM"}<small>{event.targetID || "—"}</small></td><td><code>{event.summaryJSON}</code></td><td>{new Date(event.createdAt).toLocaleString()}</td></tr>)}
      </tbody></table></div>
      {data?.manageAudit && data.manageAudit.totalPages > 1 ? <nav className="manage-pagination"><button disabled={page <= 1} onClick={() => setParameters({ page: String(page - 1) })}>Previous</button><span>{page} / {data.manageAudit.totalPages}</span><button disabled={page >= data.manageAudit.totalPages} onClick={() => setParameters({ page: String(page + 1) })}>Next</button></nav> : null}
    </section>

    {restoreTarget ? <div className="operation-confirm-backdrop" role="presentation"><section className="operation-confirm" role="dialog" aria-modal="true" aria-labelledby="restore-title"><p>DESTRUCTIVE DATABASE REPLACEMENT</p><h3 id="restore-title">Restore {restoreTarget.fileName}?</h3><ul><li>A new safety backup is created before replacement.</li><li>The selected database and managed Coser metadata replace the live state.</li><li>All sessions are revoked; executable jobs are cancelled; schedules remain paused.</li><li>Media files are never deleted or included in this operation.</li></ul><label>Type <strong>RESTORE</strong> to continue<input autoFocus value={confirmation} onChange={(event) => setConfirmation(event.target.value)} /></label><footer><button type="button" onClick={() => setRestoreTarget(null)}>Cancel</button><button className="danger" type="button" disabled={confirmation !== "RESTORE" || restoreState.loading} onClick={restore}>{restoreState.loading ? "Validating and restoring…" : "Create safety backup and restore"}</button></footer></section></div> : null}
  </main>;
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
