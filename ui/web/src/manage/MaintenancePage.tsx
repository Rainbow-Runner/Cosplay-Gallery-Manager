import { useEffect, useState } from "react";

interface MaintenanceLibrary {
  library_id: number;
  name: string;
  root_path: string;
  enabled: boolean;
}

interface MaintenanceStatus {
  state: "NORMAL" | "RESTORING" | "WAITING_VALIDATION";
  requiresValidation: boolean;
  requiresPathMapping: boolean;
  libraries: MaintenanceLibrary[];
}

interface MappingDraft {
  libraryId: number;
  expectedRootPath: string;
  rootPath: string;
  disable: boolean;
}

export function MaintenancePage() {
  const [status, setStatus] = useState<MaintenanceStatus | null>(null);
  const [mappings, setMappings] = useState<MappingDraft[]>([]);
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [message, setMessage] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [resuming, setResuming] = useState(false);

  function loadStatus() {
    return fetch("/maintenance/status", { credentials: "same-origin", cache: "no-store" })
      .then((response) => response.ok ? response.json() : Promise.reject(new Error("status unavailable")))
      .then((value: MaintenanceStatus) => {
        setStatus(value);
        setMappings(value.libraries.map((library) => ({
          libraryId: library.library_id,
          expectedRootPath: library.root_path,
          rootPath: "",
          disable: false,
        })));
      });
  }

  useEffect(() => {
    loadStatus().catch(() => setMessage("Unable to inspect maintenance state."));
  }, []);

  function updateMapping(libraryId: number, patch: Partial<MappingDraft>) {
    setMappings((current) => current.map((value) => value.libraryId === libraryId ? { ...value, ...patch } : value));
  }

  async function applyMappings() {
    setSubmitting(true);
    setMessage("");
    try {
      const response = await fetch("/maintenance/path-mappings", {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          mappings: mappings.map((value) => ({
            library_id: value.libraryId,
            expected_root_path: value.expectedRootPath,
            root_path: value.disable ? "" : value.rootPath,
            disable: value.disable,
          })),
          password,
          confirmation,
        }),
      });
      if (!response.ok) throw new Error(await response.text() || "path mapping failed");
      setPassword("");
      setConfirmation("");
      await loadStatus();
      setMessage("Media library decisions saved. No media scan was started.");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Path mapping failed.");
    } finally {
      setSubmitting(false);
    }
  }

  async function resume() {
    setResuming(true);
    setMessage("");
    try {
      const response = await fetch("/maintenance/resume", { method: "POST", credentials: "same-origin" });
      if (!response.ok) throw new Error(await response.text() || "environment validation failed");
      window.location.assign("/manage/operations");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Environment validation failed.");
      setResuming(false);
    }
  }

  const mappingReady = mappings.every((value) => value.disable || value.rootPath.trim() !== "") &&
    password !== "" && confirmation === "MAP";

  return <main className="maintenance-page"><section><p>RECOVERY MAINTENANCE</p>
    <h1>{status?.state === "WAITING_VALIDATION" ? "Restore completed" : "Maintenance in progress"}</h1>
    <p>The restored database is isolated from Browse and Manage workloads. Old sessions and executable tasks have been revoked.</p>
    {status?.requiresPathMapping ? <section className="restore-path-mapping">
      <h2>Map restored media libraries</h2>
      <p>Every restored library needs an explicit decision for this machine. Mapping changes stored paths and marks sources for reconciliation; it does not scan media.</p>
      {status.libraries.length === 0 ? <p>This backup has no media libraries. Confirm the empty mapping review to continue.</p> : null}
      {status.libraries.map((library) => {
        const draft = mappings.find((value) => value.libraryId === library.library_id);
        if (!draft) return null;
        return <fieldset key={library.library_id}>
          <legend>{library.name}</legend>
          <p>Restored root: <code>{library.root_path}</code>{library.enabled ? "" : " · previously disabled"}</p>
          <label className="field"><span>Absolute directory on this machine</span>
            <input value={draft.rootPath} disabled={draft.disable} onChange={(event) => updateMapping(library.library_id, { rootPath: event.target.value })} placeholder="/mounted/media/library" />
          </label>
          <label><input type="checkbox" checked={draft.disable} onChange={(event) => updateMapping(library.library_id, { disable: event.target.checked })} /> Disable this library on this machine</label>
        </fieldset>;
      })}
      <label className="field"><span>Owner password</span><input type="password" autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} /></label>
      <label className="field"><span>Type MAP to confirm every decision</span><input value={confirmation} onChange={(event) => setConfirmation(event.target.value)} /></label>
      <button type="button" disabled={!mappingReady || submitting} onClick={applyMappings}>{submitting ? "Saving path decisions…" : "Confirm path mappings"}</button>
    </section> : <>
      <ol><li>Confirm the configured database, cache, backup and Coser metadata locations are available.</li><li>Confirm every enabled media library and configured media tool is available.</li><li>Resume only after the current machine and mounted storage are ready.</li></ol>
      <button type="button" disabled={!status?.requiresValidation || resuming} onClick={resume}>{resuming ? "Validating environment…" : "Validate environment and resume tasks"}</button>
    </>}
    {message ? <p className="manage-message" role="alert">{message}</p> : null}
  </section></main>;
}
