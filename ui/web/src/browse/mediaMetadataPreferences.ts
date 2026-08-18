const storageKey = "cgm.mediaMetadataVisibleFields.v1";

export const mediaMetadataVisibilityKeys = [
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
    const stored = JSON.parse(window.localStorage.getItem(storageKey) ?? "null");
    if (!Array.isArray(stored)) return [...defaultMediaMetadataVisibleFields];
    return [...new Set(stored.filter((value): value is string => typeof value === "string" && allowed.has(value)))];
  } catch {
    return [...defaultMediaMetadataVisibleFields];
  }
}

export function writeMediaMetadataVisibleFields(values: string[]) {
  const normalized = [...new Set(values.filter((value) => allowed.has(value)))];
  if (normalized.length !== values.length) throw new Error("Invalid media metadata visibility selection");
  window.localStorage.setItem(storageKey, JSON.stringify(normalized));
}
