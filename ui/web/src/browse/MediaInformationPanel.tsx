import { useState } from "react";
import { useIntl } from "react-intl";
import type { MediaInformation, MediaMetadataEntry } from "./types";

const groups = ["FILE", "IMAGE", "CAMERA", "DATES", "DESCRIPTIVE", "EDITING", "VIDEO", "OTHER", "SENSITIVE"];
const categories: Record<string, string[]> = { all: groups, file: ["FILE"], dates: ["DATES"], editing: ["EDITING"], image: ["IMAGE"], camera: ["CAMERA"], video: ["VIDEO"], descriptive: ["DESCRIPTIVE"], other: ["OTHER", "SENSITIVE"] };
const labels: Record<string, string> = {
  "file.type": "type", "file.size": "size", "file.dimensions": "dimensions", "file.added_at": "addedAt", "file.name": "filename", "file.original_filename": "originalFilename", "file.source_modified": "sourceModified",
  "descriptive.title": "title", "descriptive.description": "description", "descriptive.author": "author", "descriptive.source": "source", "descriptive.software": "software", "descriptive.copyright": "copyright", "descriptive.keywords": "keywords", "descriptive.comment": "comment",
  "date.modified": "modified", "date.original": "taken", "date.digitized": "digitized", "date.created": "created", "date.metadata_modified": "metadataModified",
  "camera.make": "cameraMake", "camera.model": "cameraModel", "camera.lens": "lens", "camera.exposure_time": "exposureTime", "camera.aperture": "aperture", "camera.iso": "iso", "camera.focal_length": "focalLength", "camera.exposure_bias": "exposureBias", "camera.flash": "flash", "camera.metering_mode": "metering", "camera.white_balance": "whiteBalance",
  "image.orientation": "orientation", "image.color_space": "colorSpace", "image.resolution": "resolution", "image.icc": "icc",
  "video.duration": "duration", "video.container": "container", "video.codec": "videoCodec", "video.frame_rate": "frameRate", "video.audio_codec": "audioCodec", "sensitive.gps": "gps", "sensitive.device_identifiers": "identifier",
};

export function MediaInformationPanel({ information, loading, error, historyEnabled = true }: { information?: MediaInformation; loading: boolean; error: boolean; historyEnabled?: boolean }) {
  const intl = useIntl(); const t = (key: string) => intl.formatMessage({ id: `media.metadata.${key}` });
  const [search, setSearch] = useState(""); const [category, setCategory] = useState("all"); const [sourceFilter, setSourceFilter] = useState("all");
  const [copied, setCopied] = useState("");
  const groupOf = (e: MediaMetadataEntry) => e.visibilityKey === "file.dimensions" ? "IMAGE" : e.visibilityKey === "file.source_modified" ? "DATES" : e.visibilityKey === "descriptive.software" ? "EDITING" : e.group;
  const entries = (information?.entries ?? []).filter(e => e.visibilityKey !== "file.added_at");
  const events = historyEnabled ? information?.history ?? [] : [];
  const name = (e: MediaMetadataEntry) => labels[e.visibilityKey] ? t(labels[e.visibilityKey]) : e.label;
  const match = (values: string[]) => values.join(" ").toLocaleLowerCase().includes(search.trim().toLocaleLowerCase());
  const groupTitle = (group: string) => group === "EDITING" ? t("editing") : intl.formatMessage({ id: "media.metadataGroup." + group.toLowerCase() });
  const allEventsDated = events.length > 0 && events.every(e => e.timestamp && Number.isFinite(Date.parse(e.timestamp)));
  const ordered = allEventsDated ? [...events].sort((a,b) => Date.parse(a.timestamp)-Date.parse(b.timestamp) || a.index-b.index) : events;
  // Merge exact semantic/value/time matches, never discard original records.
  const merged = new Map<string, MediaMetadataEntry[]>();
  for (const e of entries) {
    if (sourceFilter !== "all" && e.source !== sourceFilter) continue;
    const key = JSON.stringify([groupOf(e), labels[e.visibilityKey] ? e.visibilityKey : e.key, e.value, e.timestamp ?? ""]);
    const records = merged.get(key) ?? []; records.push(e); merged.set(key, records);
  }
  const filtered = [...merged.values()].filter(records => categories[category].includes(groupOf(records[0])) && records.some(e => match([name(e),e.label,e.key,e.value,e.rawValue ?? "",e.source ?? ""])));
  const filteredEvents = categories[category].includes("EDITING") && (sourceFilter === "all" || sourceFilter === "XMP") ? ordered.filter(e => match([e.software,e.action,t("action."+e.action),e.when,e.parameters])) : [];
  const sourceName = (v?: string) => v === "FILESYSTEM" ? t("filesystem") : v === "ARCHIVE_ENTRY" ? t("archiveEntry") : v === "CGM" ? "" : v;
  async function copy(key: string,value: string) { try { await navigator.clipboard.writeText(value); setCopied(key); } catch { setCopied("failed"); } }
  const filterActive = category !== "all" || sourceFilter !== "all";
  const available = groups.filter(g => entries.some(e => groupOf(e) === g) || g === "EDITING" && events.length);
  return <section className="media-metadata-panel">
    <div className="media-metadata-tools">
      <label>{t("search")}<input type="search" value={search} onChange={e => setSearch(e.target.value)} maxLength={200} /></label>
      <details className="media-metadata-filters"><summary>{t("filters")}{filterActive ? <small>{t("filterActive")}</small> : null}</summary><div>
        <label>{t("category")}<select aria-label={t("category")} value={category} onChange={e => setCategory(e.target.value)}><option value="all">{t("category.all")}</option>{Object.entries(categories).filter(([k,v]) => k !== "all" && (k === category || v.some(g => available.includes(g)))).map(([k]) => <option key={k} value={k}>{t("category."+k)}</option>)}</select></label>
        <label>{t("sourceFilter")}<select aria-label={t("sourceFilter")} value={sourceFilter} onChange={e => setSourceFilter(e.target.value)}><option value="all">{t("category.all")}</option>{[...new Set([...entries.map(e=>e.source).filter((v): v is string=>!!v),...(events.length ? ["XMP"] : []),...(sourceFilter !== "all" ? [sourceFilter] : [])])].filter(v=>v !== "CGM").map(v => <option key={v} value={v}>{sourceName(v)}</option>)}</select></label>
      </div></details>
    </div>
    {loading ? <p role="status">{intl.formatMessage({id:"media.metadataPending"})}</p> : null}
    {error || information?.state === "ERROR" ? <p role="status">{intl.formatMessage({id:"media.metadataError"})}</p> : null}
    {information?.truncated ? <p className="media-metadata-warning" role="status">{t("truncated")}</p> : null}
    {[...new Set(information?.warnings ?? [])].map(w => <p className="media-metadata-warning" role="status" key={w}>{t(w === "MODIFIED_TIME_CONFLICT" ? "conflict" : "partial")}</p>)}
    {groups.map(group => {
      const items = filtered.filter(records => groupOf(records[0]) === group);
      const history = group === "EDITING" && filteredEvents.length > 0;
      return items.length || history ? <details className="media-metadata-section" key={group} open={["FILE","IMAGE","DATES","EDITING"].includes(group) || !!search || filterActive}>
        <summary>{groupTitle(group)}<small>{items.length + (history ? filteredEvents.length : 0)}</small></summary>
        {items.map(records => {
          const e = records[0], sources = [...new Set(records.map(r=>sourceName(r.source)).filter(Boolean))].join(" / ");
          const unzoned = group === "DATES" && /\d{2}:\d{2}/.test(e.value) && records.some(r=>!r.timestamp);
          return <details className="media-metadata-row" key={e.key} data-property={e.visibilityKey}>
            <summary title={t("expandRecord")}><span className="media-metadata-label">{name(e)}{sources ? <small className="media-metadata-source">{sources}</small> : null}</span><span className="media-metadata-value"><span>{e.value}</span>{unzoned ? <small className="media-metadata-indicator" aria-label={t("unzoned")} title={t("unzoned")}>⚠</small> : null}</span></summary>
            <div className="media-metadata-records">{unzoned ? <p>{t("unzoned")}</p> : null}{records.map(r => <div className="media-metadata-raw" key={r.key}>
              <header><small>{sourceName(r.source)}</small><code>{r.key}</code><button type="button" onClick={() => void copy(r.key,r.rawValue || r.value)} aria-label={t("copy")+" · "+r.key}>{t(copied === r.key ? "copied" : "copy")}</button></header>
              {r.rawValue && r.rawValue !== r.value ? <code className="media-metadata-original">{r.rawValue}</code> : null}
            </div>)}{e.visibilityKey === "file.original_filename" ? <p className="media-metadata-note">{t("filenameNote")}</p> : null}</div>
          </details>;
        })}
        {history ? <><p className="media-metadata-note">{t(allEventsDated ? "timeOrder" : "sourceOrder")}</p><ol className="media-metadata-history">{filteredEvents.map(e => <li key={e.index}><details><summary><span><strong>{t("action."+e.action)}</strong>{e.software ? <span>{e.software}</span> : null}</span><time>{e.when || t("unknownTime")}</time></summary><div className="media-metadata-event-detail"><small>#{e.index} · XMP</small>{e.when && !e.timestamp ? <p>{t("unzoned")}</p> : null}{e.parameters ? <p>{e.parameters}</p> : null}<button type="button" onClick={() => void copy("history."+e.index,[e.action,e.when,e.software,e.parameters].filter(Boolean).join("\n"))}>{t(copied === "history."+e.index ? "copied" : "copy")}</button></div></details></li>)}</ol></> : null}
      </details> : null;
    })}
    {!events.length && historyEnabled && information?.state === "READY" && category === "all" && !search && sourceFilter === "all" ? <p className="media-metadata-note">{t("noHistory")}</p> : null}
    {!loading && information?.state === "READY" && !filtered.length && !filteredEvents.length ? <p role="status">{t("noMatch")}</p> : null}
    {copied === "failed" ? <p role="status">{t("copyFailed")}</p> : null}
  </section>;
}
