import { useMutation, useQuery } from "@apollo/client/react";
import { type FormEvent, useEffect, useState } from "react";
import { MANAGE_RUNTIME_SETTINGS, UPDATE_RUNTIME_SETTINGS } from "../api/manage";
import type { ManageCacheStorage, ManageRuntimeSettings, ManageVideoDependencyStatus } from "./types";

export function ManageSettingsPage() {
  const query = useQuery<{ manageRuntimeSettings: ManageRuntimeSettings; manageCacheStorage: ManageCacheStorage; manageVideoDependencyStatus: ManageVideoDependencyStatus }>(MANAGE_RUNTIME_SETTINGS);
  const [update, updateState] = useMutation<{ updateRuntimeSettings: ManageRuntimeSettings }>(UPDATE_RUNTIME_SETTINGS);
  const [draft, setDraft] = useState<ManageRuntimeSettings | null>(null); const [message, setMessage] = useState("");
  useEffect(() => { if (query.data) setDraft(query.data.manageRuntimeSettings); }, [query.data]);
  if (!draft) return <main className="manage-page"><p className="state-message">{query.error ? "Unable to load settings." : "Loading…"}</p></main>;
  const current = draft;
  const input = { ...draft } as Partial<ManageRuntimeSettings>; delete input.settingsRevision;
  const validRange = (value: number, minimum: number, maximum?: number) => Number.isFinite(value) && value >= minimum && (maximum === undefined || value <= maximum);
  const settingsValid =
    validRange(draft.relatedLimit, 1, 24) && validRange(draft.tagParentWeight, 0, 1) &&
    validRange(draft.tagMinimumScore, 0, 1) && validRange(draft.tagMaximumDepth, 0, 10) &&
    validRange(draft.randomLimit, 1, 100) && validRange(draft.randomStaticQuota, 0, 1) &&
    validRange(draft.randomGIFQuota, 0, 1) && validRange(draft.randomVideoQuota, 0, 1) &&
    validRange(draft.randomGalleryRepeatDecay, 0, 1) && validRange(draft.enhancedCacheMaximumBytes, 0) &&
    validRange(draft.minimumFreeBytes, 0) && validRange(draft.minimumFreePercent, 0, 1) &&
    validRange(draft.dailyBackupRetention, 1, 365) && validRange(draft.archiveMaxEntries, 1) &&
    validRange(draft.archiveMaxEntryBytes, 1) && validRange(draft.archiveMaxTotalBytes, 1) &&
    validRange(draft.archiveMaxCompressionRatio, 1) && validRange(draft.archiveMaxImagePixels, 1);
  async function submit(event: FormEvent) { event.preventDefault(); if (!settingsValid) return; setMessage(""); try { const result = await update({ variables: { expectedSettingsRevision: current.settingsRevision, input } }); if (!result.data) throw new Error("Server did not return the saved settings"); setDraft(result.data.updateRuntimeSettings); setMessage("Settings saved"); } catch (error) { setMessage(error instanceof Error ? error.message : "Settings revision conflict"); } }
  const number = (key: keyof ManageRuntimeSettings, value: string) => setDraft({ ...draft, [key]: Number(value) });
  const toggle = (key: keyof ManageRuntimeSettings, checked: boolean) => setDraft({ ...draft, [key]: checked });
  return <main className="manage-page"><header className="manage-heading"><div><p>RUNTIME · REVISION {draft.settingsRevision}</p><h2>Settings</h2></div></header>{message ? <p className="manage-message" role="status">{message}</p> : null}<form className="settings-form" onSubmit={submit}>
    <fieldset><legend>Browse & controls</legend><label>Home source<select value={draft.homeScope} onChange={(event) => setDraft({ ...draft, homeScope: event.target.value as ManageRuntimeSettings["homeScope"] })}><option value="LIST">LIST</option><option value="MAGIC">MAGIC</option><option value="ALL">ALL</option></select></label><Check label="Gallery Poster Scrubber" checked={draft.galleryCardScrubberEnabled} change={(value) => toggle("galleryCardScrubberEnabled", value)} /><Check label="Gallery detail media filter" checked={draft.galleryDetailMediaFilterEnabled} change={(value) => toggle("galleryDetailMediaFilterEnabled", value)} /><Check label="Gallery card controls" checked={draft.galleryCardControlsVisible} change={(value) => toggle("galleryCardControlsVisible", value)} /><Check label="Media card controls" checked={draft.mediaCardControlsVisible} change={(value) => toggle("mediaCardControlsVisible", value)} /><Check label="Detail personal controls" checked={draft.detailPersonalControlsVisible} change={(value) => toggle("detailPersonalControlsVisible", value)} /></fieldset>
    <fieldset><legend>Related & random</legend><NumberField label="Related limit" value={draft.relatedLimit} min={1} max={24} change={(value) => number("relatedLimit", value)} /><NumberField label="Tag parent weight" value={draft.tagParentWeight} min={0} max={1} step={0.01} change={(value) => number("tagParentWeight", value)} /><NumberField label="Tag minimum score" value={draft.tagMinimumScore} min={0} max={1} step={0.01} change={(value) => number("tagMinimumScore", value)} /><NumberField label="Tag ancestor depth" value={draft.tagMaximumDepth} min={0} max={10} change={(value) => number("tagMaximumDepth", value)} /><NumberField label="Random items" value={draft.randomLimit} min={1} max={100} change={(value) => number("randomLimit", value)} /><NumberField label="Static quota" value={draft.randomStaticQuota} min={0} max={1} step={0.01} change={(value) => number("randomStaticQuota", value)} /><NumberField label="GIF quota" value={draft.randomGIFQuota} min={0} max={1} step={0.01} change={(value) => number("randomGIFQuota", value)} /><NumberField label="Video quota" value={draft.randomVideoQuota} min={0} max={1} step={0.01} change={(value) => number("randomVideoQuota", value)} /><NumberField label="Gallery repeat decay" value={draft.randomGalleryRepeatDecay} min={0} max={1} step={0.01} change={(value) => number("randomGalleryRepeatDecay", value)} /></fieldset>
    <fieldset className="settings-cache-status"><legend>Generated cache</legend><dl><dt>Location</dt><dd><code>{query.data?.manageCacheStorage.path ?? "Unavailable"}</code></dd><dt>Occupied space</dt><dd>{formatStorageBytes(query.data?.manageCacheStorage.byteSize ?? 0)}<small>{(query.data?.manageCacheStorage.byteSize ?? 0).toLocaleString()} bytes · {(query.data?.manageCacheStorage.fileCount ?? 0).toLocaleString()} files</small></dd><dt>Permanent base</dt><dd>{formatStorageBytes(query.data?.manageCacheStorage.baseByteSize ?? 0)}</dd><dt>Reclaimable</dt><dd>{formatStorageBytes(query.data?.manageCacheStorage.enhancedByteSize ?? 0)}</dd></dl><p>The location is read-only. CARD_480 and static posters form the permanent base; on-demand Lightbox images are reclaimable.</p></fieldset>
    <VideoDependencyPanel value={query.data?.manageVideoDependencyStatus} />
    <fieldset><legend>Storage & schedules</legend><NumberField label="Reclaimable cache limit (GiB)" value={bytesToGiB(draft.enhancedCacheMaximumBytes)} min={0.25} step={0.25} change={(value) => setDraft({ ...draft, enhancedCacheMaximumBytes: gibToBytes(value) })} /><NumberField label="Minimum free bytes" value={draft.minimumFreeBytes} min={0} change={(value) => number("minimumFreeBytes", value)} /><NumberField label="Minimum free ratio" value={draft.minimumFreePercent} min={0} max={1} step={0.01} change={(value) => number("minimumFreePercent", value)} /><Check label="Automatic scan enabled" checked={draft.automaticScanEnabled} change={(value) => toggle("automaticScanEnabled", value)} /><Check label="Daily database snapshot" checked={draft.dailyBackupEnabled} change={(value) => toggle("dailyBackupEnabled", value)} /><NumberField label="Daily snapshots retained" value={draft.dailyBackupRetention} min={1} max={365} change={(value) => number("dailyBackupRetention", value)} /><Check label="Automatic schedules suspended" checked={draft.automaticSchedulesSuspended} change={(value) => toggle("automaticSchedulesSuspended", value)} /></fieldset>
    <fieldset><legend>ZIP / CBZ resource limits</legend><p>Structural path and archive safety checks can never be disabled.</p><NumberField label="Maximum entries" value={draft.archiveMaxEntries} min={1} change={(value) => number("archiveMaxEntries", value)} /><NumberField label="Maximum entry bytes" value={draft.archiveMaxEntryBytes} min={1} change={(value) => number("archiveMaxEntryBytes", value)} /><NumberField label="Maximum total bytes" value={draft.archiveMaxTotalBytes} min={1} change={(value) => number("archiveMaxTotalBytes", value)} /><NumberField label="Maximum compression ratio" value={draft.archiveMaxCompressionRatio} min={1} step={1} change={(value) => number("archiveMaxCompressionRatio", value)} /><NumberField label="Maximum decoded pixels" value={draft.archiveMaxImagePixels} min={1} change={(value) => number("archiveMaxImagePixels", value)} /></fieldset>
    <footer><button type="submit" disabled={!settingsValid || updateState.loading}>{updateState.loading ? "Saving…" : "Save all runtime settings"}</button></footer></form></main>;
}

function VideoDependencyPanel({ value }: { value?: ManageVideoDependencyStatus }) {
  const tool = (available: boolean, source: string, version: string, error: string) => available
    ? <><strong>Available</strong><small>{version || "Unknown version"} · {source || "Unknown source"}</small></>
    : <><strong>Unavailable</strong><small>{error || "DEPENDENCY_UNAVAILABLE"}</small></>;
  return <fieldset className="settings-cache-status"><legend>Video processing</legend><dl>
    <dt>FFmpeg</dt><dd>{value ? tool(value.ffmpegAvailable,value.ffmpegSource,value.ffmpegVersion,value.ffmpegErrorCode) : "Checking…"}</dd>
    <dt>FFprobe</dt><dd>{value ? tool(value.ffprobeAvailable,value.ffprobeSource,value.ffprobeVersion,value.ffprobeErrorCode) : "Checking…"}</dd>
  </dl><p>Paths come from startup configuration or the local executable search. This page does not download or modify media tools.</p></fieldset>;
}

function Check({ label, checked, change }: { label: string; checked: boolean; change: (value: boolean) => void }) { return <label className="settings-check"><input type="checkbox" checked={checked} onChange={(event) => change(event.target.checked)} />{label}</label>; }
function NumberField({ label, value, change, min, max, step }: { label: string; value: number; change: (value: string) => void; min: number; max?: number; step?: number }) { return <label>{label}<input type="number" value={value} min={min} max={max} step={step} onChange={(event) => change(event.target.value)} /></label>; }
export function formatStorageBytes(value: number) {
  if (!Number.isFinite(value) || value <= 0) return "0 B";
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  const unit = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1);
  const amount = value / 1024 ** unit;
  return `${amount.toFixed(unit === 0 || amount >= 100 ? 0 : amount >= 10 ? 1 : 2)} ${units[unit]}`;
}

export function bytesToGiB(value?: number) { return Number.isFinite(value) ? value! / 1024 ** 3 : 50; }
export function gibToBytes(value: string) { return Math.round(Number(value) * 1024 ** 3); }
