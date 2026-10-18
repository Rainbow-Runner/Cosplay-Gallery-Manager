import type { BrowseGalleryCard } from "./types";

type GalleryMediaCounts = BrowseGalleryCard["media"];

const mediaTypes = [
  ["photo", "P"],
  ["selfie", "S"],
  ["gif", "G"],
  ["video", "V"],
] as const satisfies ReadonlyArray<readonly [keyof GalleryMediaCounts, string]>;

export function formatGalleryMediaCount(media: GalleryMediaCounts): string {
  return mediaTypes
    .filter(([kind]) => media[kind] > 0)
    .map(([kind, suffix]) => `${media[kind]}${suffix}`)
    .join(" ");
}
