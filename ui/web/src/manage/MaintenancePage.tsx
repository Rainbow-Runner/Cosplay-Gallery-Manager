import { useEffect, useState } from "react";

interface MaintenanceStatus { state: "NORMAL" | "RESTORING" | "WAITING_VALIDATION"; requiresValidation: boolean }

export function MaintenancePage() {
  const [status, setStatus] = useState<MaintenanceStatus | null>(null);
  const [message, setMessage] = useState("");
  const [resuming, setResuming] = useState(false);

  useEffect(() => {
    fetch("/maintenance/status", { credentials: "same-origin", cache: "no-store" })
      .then((response) => response.ok ? response.json() : Promise.reject(new Error("status unavailable")))
      .then(setStatus)
      .catch(() => setMessage("Unable to inspect maintenance state."));
  }, []);

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

  return <main className="maintenance-page"><section><p>RECOVERY MAINTENANCE</p><h1>{status?.state === "WAITING_VALIDATION" ? "Restore completed" : "Maintenance in progress"}</h1>
    <p>The restored database is isolated from Browse and Manage workloads. Old sessions and executable tasks have been revoked.</p>
    <ol><li>Confirm the configured database, cache, backup and Coser metadata locations are available.</li><li>Confirm the configured media tools can be executed.</li><li>Resume only after the current machine and mounted storage are ready.</li></ol>
    {message ? <p className="manage-message" role="alert">{message}</p> : null}
    <button type="button" disabled={!status?.requiresValidation || resuming} onClick={resume}>{resuming ? "Validating environment…" : "Validate environment and resume tasks"}</button>
  </section></main>;
}
