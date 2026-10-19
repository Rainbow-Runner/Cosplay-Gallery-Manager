const storageKey = "cgm.mediaMetadataVisibleFields.v2";
const newMetadataKeys = ["file.name", "file.original_filename", "file.source_modified", "date.created", "date.metadata_modified", "history.edits", "history.conversions", "image.icc"] as const;

export const mediaMetadataVisibilityKeys = [
  ...newMetadataKeys,
  "file.type", "file.size", "file.dimensions", "file.added_at",
  "descriptive.title", "descriptive.description", "descriptive.author", "descriptive.source", "descriptive.software", "descriptive.copyright", "descriptive.keywords", "descriptive.comment",
  "date.modified", "date.original", "date.digitized",
  "camera.make", "camera.model", "camera.lens", "camera.exposure_time", "camera.aperture", "camera.iso", "camera.focal_length", "camera.exposure_bias", "camera.flash", "camera.metering_mode", "camera.white_balance",
  "image.orientation", "image.color_space", "image.resolution",
  "video.duration", "video.container", "video.codec", "video.frame_rate", "video.audio_codec", "other.exif",
  "sensitive.gps", "sensitive.device_identifiers",
] as const;

const allowed = new Set<string>(mediaMetadataVisibilityKeys);
export const defaultMediaMetadataVisibleFields = mediaMetadataVisibilityKeys.filter((key) => !key.startsWith("sensitive."));

export function readMediaMetadataVisibleFields(): string[] {
  if (typeof window === "undefined") return [...defaultMediaMetadataVisibleFields];
  try {
    const current = window.localStorage.getItem(storageKey);
    const stored = JSON.parse(current ?? window.localStorage.getItem("cgm.mediaMetadataVisibleFields.v1") ?? "null");
    if (!Array.isArray(stored)) return [...defaultMediaMetadataVisibleFields];
    const valid = stored.filter((value): value is string => typeof value === "string" && allowed.has(value));
    // Preserve old exclusions, including an explicit all-off choice. Only the
    // newly introduced non-sensitive controls get their default on migration.
    return [...new Set([...valid, ...(current === null && valid.length ? newMetadataKeys : [])])];
  } catch {
    return [...defaultMediaMetadataVisibleFields];
  }
}

export function writeMediaMetadataVisibleFields(values: string[]) {
  const normalized = [...new Set(values.filter((value) => allowed.has(value)))];
  if (normalized.length !== values.length) throw new Error("Invalid media metadata visibility selection");
  window.localStorage.setItem(storageKey, JSON.stringify(normalized));
}
