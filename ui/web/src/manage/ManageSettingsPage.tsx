import { useMutation, useQuery } from "@apollo/client/react";
import { type FormEvent, useEffect, useState } from "react";
import { MANAGE_RUNTIME_SETTINGS, UPDATE_RUNTIME_SETTINGS } from "../api/manage";
import { mediaMetadataVisibilityKeys, readMediaMetadataVisibleFields, writeMediaMetadataVisibleFields } from "../browse/mediaMetadataPreferences";
import type { ManageCacheStorage, ManageRuntimeSettings, ManageVideoDependencyStatus } from "./types";

export function ManageSettingsPage() {
  const query = useQuery<{ manageRuntimeSettings: ManageRuntimeSettings; manageCacheStorage: ManageCacheStorage; manageVideoDependencyStatus: ManageVideoDependencyStatus }>(MANAGE_RUNTIME_SETTINGS);
  const [update, updateState] = useMutation<{ updateRuntimeSettings: ManageRuntimeSettings }>(UPDATE_RUNTIME_SETTINGS);
  const [draft, setDraft] = useState<ManageRuntimeSettings | null>(null); const [message, setMessage] = useState("");
  const [visibleMetadataFields, setVisibleMetadataFields] = useState(readMediaMetadataVisibleFields);
  const [metadataMessage, setMetadataMessage] = useState("");
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
	validRange(draft.galleryAnimatedPlaybackLimit, 1, 16) && validRange(draft.galleryAnimatedLockIntervalMS, 700, 1000) &&
    validRange(draft.minimumFreeBytes, 0) && validRange(draft.minimumFreePercent, 0, 1) &&
	validRange(draft.automaticScanIntervalMinutes, 15, 10080) &&
    validRange(draft.dailyBackupRetention, 1, 365) && validRange(draft.archiveMaxEntries, 1) &&
    validRange(draft.archiveMaxEntryBytes, 1) && validRange(draft.archiveMaxTotalBytes, 1) &&
    validRange(draft.archiveMaxCompressionRatio, 1) && validRange(draft.archiveMaxImagePixels, 1);
  async function submit(event: FormEvent) { event.preventDefault(); if (!settingsValid) return; setMessage(""); try { const result = await update({ variables: { expectedSettingsRevision: current.settingsRevision, input } }); if (!result.data) throw new Error("Server did not return the saved settings"); setDraft(result.data.updateRuntimeSettings); setMessage("Settings saved"); } catch (error) { setMessage(error instanceof Error ? error.message : "Settings revision conflict"); } }
  const number = (key: keyof ManageRuntimeSettings, value: string) => setDraft({ ...draft, [key]: Number(value) });
  const toggle = (key: keyof ManageRuntimeSettings, checked: boolean) => setDraft({ ...draft, [key]: checked });
  const toggleMetadata = (key: string, checked: boolean) => setVisibleMetadataFields(checked
    ? [...visibleMetadataFields, key] : visibleMetadataFields.filter((value) => value !== key));
  const saveMetadata = () => { try { writeMediaMetadataVisibleFields(visibleMetadataFields); setMetadataMessage("当前浏览器的媒体元数据显示设置已保存 / Saved for this browser"); } catch (error) { setMetadataMessage(error instanceof Error ? error.message : "Unable to save metadata visibility"); } };
  return <main className="manage-page"><header className="manage-heading"><div><p>RUNTIME · REVISION {draft.settingsRevision}</p><h2>Settings</h2></div></header>{message ? <p className="manage-message" role="status">{message}</p> : null}<form className="settings-form" onSubmit={submit}>
    <fieldset><legend>Browse & controls</legend><label>Home source<select value={draft.homeScope} onChange={(event) => setDraft({ ...draft, homeScope: event.target.value as ManageRuntimeSettings["homeScope"] })}><option value="LIST">LIST</option><option value="MAGIC">MAGIC</option><option value="ALL">ALL</option></select></label><NumberField label="Animated playback limit" value={draft.galleryAnimatedPlaybackLimit} min={1} max={16} change={(value) => number("galleryAnimatedPlaybackLimit", value)} /><NumberField label="Animation lock interval (ms)" value={draft.galleryAnimatedLockIntervalMS} min={700} max={1000} step={50} change={(value) => number("galleryAnimatedLockIntervalMS", value)} /><Check label="Gallery Poster Scrubber" checked={draft.galleryCardScrubberEnabled} change={(value) => toggle("galleryCardScrubberEnabled", value)} /><Check label="Gallery detail media filter" checked={draft.galleryDetailMediaFilterEnabled} change={(value) => toggle("galleryDetailMediaFilterEnabled", value)} /><Check label="Gallery card controls" checked={draft.galleryCardControlsVisible} change={(value) => toggle("galleryCardControlsVisible", value)} /><Check label="Media card controls" checked={draft.mediaCardControlsVisible} change={(value) => toggle("mediaCardControlsVisible", value)} /><Check label="Detail personal controls" checked={draft.detailPersonalControlsVisible} change={(value) => toggle("detailPersonalControlsVisible", value)} /></fieldset>
    <fieldset><legend>Related & random</legend><NumberField label="Related limit" value={draft.relatedLimit} min={1} max={24} change={(value) => number("relatedLimit", value)} /><NumberField label="Tag parent weight" value={draft.tagParentWeight} min={0} max={1} step={0.01} change={(value) => number("tagParentWeight", value)} /><NumberField label="Tag minimum score" value={draft.tagMinimumScore} min={0} max={1} step={0.01} change={(value) => number("tagMinimumScore", value)} /><NumberField label="Tag ancestor depth" value={draft.tagMaximumDepth} min={0} max={10} change={(value) => number("tagMaximumDepth", value)} /><NumberField label="Random items" value={draft.randomLimit} min={1} max={100} change={(value) => number("randomLimit", value)} /><NumberField label="Static quota" value={draft.randomStaticQuota} min={0} max={1} step={0.01} change={(value) => number("randomStaticQuota", value)} /><NumberField label="GIF quota" value={draft.randomGIFQuota} min={0} max={1} step={0.01} change={(value) => number("randomGIFQuota", value)} /><NumberField label="Video quota" value={draft.randomVideoQuota} min={0} max={1} step={0.01} change={(value) => number("randomVideoQuota", value)} /><NumberField label="Gallery repeat decay" value={draft.randomGalleryRepeatDecay} min={0} max={1} step={0.01} change={(value) => number("randomGalleryRepeatDecay", value)} /></fieldset>
    <fieldset className="settings-cache-status"><legend>Generated cache</legend><dl><dt>Location</dt><dd><code>{query.data?.manageCacheStorage.path ?? "Unavailable"}</code></dd><dt>Occupied space</dt><dd>{formatStorageBytes(query.data?.manageCacheStorage.byteSize ?? 0)}<small>{(query.data?.manageCacheStorage.byteSize ?? 0).toLocaleString()} bytes · {(query.data?.manageCacheStorage.fileCount ?? 0).toLocaleString()} files</small></dd><dt>Permanent base</dt><dd>{formatStorageBytes(query.data?.manageCacheStorage.baseByteSize ?? 0)}</dd><dt>Reclaimable</dt><dd>{formatStorageBytes(query.data?.manageCacheStorage.enhancedByteSize ?? 0)}</dd></dl><p>The location is read-only. CARD_480 and static posters form the permanent base; on-demand Lightbox images are reclaimable.</p></fieldset>
    <VideoDependencyPanel value={query.data?.manageVideoDependencyStatus} />
    <MetadataVisibilityPanel selected={visibleMetadataFields} toggle={toggleMetadata} save={saveMetadata} message={metadataMessage} />
    <fieldset><legend>存储与计划 / Storage & schedules</legend><NumberField label="可回收缓存上限（GiB）/ Reclaimable cache limit" value={bytesToGiB(draft.enhancedCacheMaximumBytes)} min={0.25} step={0.25} change={(value) => setDraft({ ...draft, enhancedCacheMaximumBytes: gibToBytes(value) })} /><NumberField label="最小可用字节 / Minimum free bytes" value={draft.minimumFreeBytes} min={0} change={(value) => number("minimumFreeBytes", value)} /><NumberField label="最小可用比例 / Minimum free ratio" value={draft.minimumFreePercent} min={0} max={1} step={0.01} change={(value) => number("minimumFreePercent", value)} /><Check label="启用自动扫描 / Enable automatic scans" checked={draft.automaticScanEnabled} change={(value) => toggle("automaticScanEnabled", value)} />{draft.automaticScanEnabled ? <><Check label="服务启动后扫描一次 / Scan once after service startup" checked={draft.automaticScanOnStartup} change={(value) => toggle("automaticScanOnStartup", value)} /><NumberField label="扫描周期（分钟）/ Scan interval (minutes)" value={draft.automaticScanIntervalMinutes} min={15} max={10080} step={15} change={(value) => number("automaticScanIntervalMinutes", value)} /><p>计划每分钟检查一次到期状态；多个进程或重启不会重复执行同一周期。最短15分钟，最长7天。Scheduled work checks once per minute and uses a persistent lease.</p></> : null}<Check label="每日数据库快照 / Daily database snapshot" checked={draft.dailyBackupEnabled} change={(value) => toggle("dailyBackupEnabled", value)} /><NumberField label="快照保留数量 / Daily snapshots retained" value={draft.dailyBackupRetention} min={1} max={365} change={(value) => number("dailyBackupRetention", value)} /><Check label="暂停所有自动计划 / Suspend all automatic schedules" checked={draft.automaticSchedulesSuspended} change={(value) => toggle("automaticSchedulesSuspended", value)} /></fieldset>
    <fieldset><legend>ZIP / CBZ resource limits</legend><p>Structural path and archive safety checks can never be disabled.</p><NumberField label="Maximum entries" value={draft.archiveMaxEntries} min={1} change={(value) => number("archiveMaxEntries", value)} /><NumberField label="Maximum entry bytes" value={draft.archiveMaxEntryBytes} min={1} change={(value) => number("archiveMaxEntryBytes", value)} /><NumberField label="Maximum total bytes" value={draft.archiveMaxTotalBytes} min={1} change={(value) => number("archiveMaxTotalBytes", value)} /><NumberField label="Maximum compression ratio" value={draft.archiveMaxCompressionRatio} min={1} step={1} change={(value) => number("archiveMaxCompressionRatio", value)} /><NumberField label="Maximum decoded pixels" value={draft.archiveMaxImagePixels} min={1} change={(value) => number("archiveMaxImagePixels", value)} /></fieldset>
    <footer><button type="submit" disabled={!settingsValid || updateState.loading}>{updateState.loading ? "Saving…" : "Save all runtime settings"}</button></footer></form></main>;
}

const metadataVisibilityGroups = [
  { name: "文件 / File", fields: [["file.type","媒体类型 / Media type"],["file.size","文件大小 / File size"],["file.dimensions","尺寸 / Dimensions"],["file.added_at","CGM 收录时间 / Added to CGM"]] },
  { name: "描述 / Description", fields: [["descriptive.title","标题 / Title"],["descriptive.description","描述 / Description"],["descriptive.author","作者 / Author"],["descriptive.source","来源 / Source"],["descriptive.software","程序名称 / Software"],["descriptive.copyright","版权 / Copyright"],["descriptive.keywords","关键词 / Keywords"],["descriptive.comment","备注 / Comment"]] },
  { name: "日期 / Dates", fields: [["date.modified","修改日期 / Modified"],["date.original","拍摄日期 / Date taken"],["date.digitized","获取／数字化日期 / Digitized"]] },
  { name: "相机 / Camera", fields: [["camera.make","制造商 / Manufacturer"],["camera.model","型号 / Model"],["camera.lens","镜头 / Lens"],["camera.exposure_time","曝光时间 / Exposure"],["camera.aperture","光圈值 / Aperture"],["camera.iso","ISO 感光度"],["camera.focal_length","焦距 / Focal length"],["camera.exposure_bias","曝光补偿 / Exposure bias"],["camera.flash","闪光灯 / Flash"],["camera.metering_mode","测光模式 / Metering"],["camera.white_balance","白平衡 / White balance"]] },
  { name: "图像与视频 / Image & video", fields: [["image.orientation","方向 / Orientation"],["image.color_space","色彩空间 / Colour space"],["image.resolution","分辨率 / Resolution"],["video.duration","视频时长 / Duration"],["video.container","封装格式 / Container"],["video.codec","视频编码 / Video codec"],["video.frame_rate","帧率 / Frame rate"],["video.audio_codec","音频编码 / Audio codec"],["other.exif","其他安全 EXIF 项 / Other safe EXIF"]] },
  { name: "敏感信息（默认关闭） / Sensitive (off by default)", sensitive: true, fields: [["sensitive.gps","GPS 位置元数据"],["sensitive.device_identifiers","设备／图像唯一标识"]] },
] as const;
const metadataVisibilityKeySet = new Set<string>(mediaMetadataVisibilityKeys);

function MetadataVisibilityPanel({ selected, toggle, save, message }: { selected: string[]; toggle: (key: string, checked: boolean) => void; save: () => void; message: string }) {
  const enabled = new Set(selected);
  return <fieldset className="settings-metadata-visibility"><legend>媒体详情元数据 / Media detail metadata</legend>
    <p>详情页会按需只读原始媒体，不写入数据库或缓存。此选择仅保存在当前浏览器，不包含在完整备份中；服务端仍会校验并过滤每次请求。嵌入缩略图、MakerNote、二进制块和物理路径不会被提取，GPS 与唯一标识需要明确启用。</p>
    {metadataVisibilityGroups.map((group) => <section className={"sensitive" in group && group.sensitive ? "is-sensitive" : ""} key={group.name}><h3>{group.name}</h3>{group.fields.map(([key,label]) => <Check key={key} label={label} checked={enabled.has(key)} change={(checked) => metadataVisibilityKeySet.has(key) && toggle(key,checked)} />)}</section>)}
    <button className="settings-metadata-visibility__save" type="button" onClick={save}>保存元数据显示设置 / Save metadata visibility</button>{message ? <p role="status">{message}</p> : null}
  </fieldset>;
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
