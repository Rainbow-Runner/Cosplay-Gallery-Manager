import type { GalleryMember } from "./types";

export type GalleryMediaLayout = "GRID" | "JUSTIFIED";
export const galleryMediaLayoutStorageKey = "cgm.gallery.media-layout";

export function readGalleryMediaLayout(): GalleryMediaLayout {
  try { return window.localStorage.getItem(galleryMediaLayoutStorageKey) === "JUSTIFIED" ? "JUSTIFIED" : "GRID"; }
  catch { return "GRID"; }
}

export function saveGalleryMediaLayout(layout: GalleryMediaLayout) {
  try { window.localStorage.setItem(galleryMediaLayoutStorageKey, layout); }
  catch { /* Layout switching remains usable when storage is unavailable. */ }
}

export interface MediaTileBox { left: number; top: number; width: number; height: number }

// Preserve DOM/business order. Extreme proportions get a bounded box with
// contain/letterboxing, never a crop or a reordered dense grid.
export function justifiedMediaLayout(items: GalleryMember[], width: number, gap = 12) {
  const boxes = new Map<string, MediaTileBox>();
  if (!Number.isFinite(width) || width <= 0 || items.length === 0) return { boxes, height: 0 };
  gap = Math.max(0, Number.isFinite(gap) ? gap : 12);
  const targetHeight = 240;
  let top = 0;
  let row: { item: GalleryMember; ratio: number }[] = [];
  const fitHeight = (values: typeof row) => Math.max(0, width - gap * (values.length - 1)) / values.reduce((sum, entry) => sum + entry.ratio, 0);
  const flush = (last: boolean) => {
    const height = Math.min(fitHeight(row), 300, 480 / Math.max(...row.map((entry) => entry.ratio)), last ? targetHeight : Infinity);
    let left = 0;
    for (const { item, ratio } of row) {
      const tileWidth = ratio * height;
      boxes.set(item.itemUUID, { left, top, width: tileWidth, height });
      left += tileWidth + gap;
    }
    top += height + gap;
    row = [];
  };
  for (const item of items) {
    const ratio = item.previewWidth && item.previewHeight && item.previewWidth > 0 && item.previewHeight > 0
      ? item.previewWidth / item.previewHeight : 3 / 4;
    row.push({ item, ratio: Number.isFinite(ratio) ? Math.min(2.4, Math.max(0.35, ratio)) : 3 / 4 });
    if (fitHeight(row) <= targetHeight) {
      const previous = row.slice(0, -1);
      if (previous.length && fitHeight(previous) <= 300 && Math.abs(fitHeight(previous) - targetHeight) < Math.abs(fitHeight(row) - targetHeight)) {
        const carry = row.pop()!;
        flush(false);
        row = [carry];
      } else flush(false);
    }
  }
  if (row.length) flush(true);
  return { boxes, height: Math.max(0, top - gap) };
}
