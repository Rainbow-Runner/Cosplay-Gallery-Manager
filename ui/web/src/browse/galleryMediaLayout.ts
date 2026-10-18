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

// Use the measured content width, not screen size or device pixel ratio.
export function justifiedTargetHeight(width: number) {
  const anchors = [[0, 200], [600, 240], [1024, 320], [1600, 380], [2400, 440]];
  for (let i = 1; i < anchors.length; i++) {
    const [right, height] = anchors[i], [left, previous] = anchors[i - 1];
    if (width <= right) return previous + (height - previous) * Math.max(0, width - left) / (right - left);
  }
  return 440;
}

// Preserve DOM/business order. Extreme proportions get a bounded box with
// contain/letterboxing, never a crop or a reordered dense grid.
export function justifiedMediaLayout(items: GalleryMember[], width: number, gap = 12) {
  const boxes = new Map<string, MediaTileBox>();
  if (!Number.isFinite(width) || width <= 0 || items.length === 0) return { boxes, height: 0 };
  gap = Math.max(0, Number.isFinite(gap) ? gap : 12);
  const targetHeight = justifiedTargetHeight(width);
  const minHeight = targetHeight * 0.85, maxHeight = targetHeight * 1.15;
  let top = 0;
  let row: { item: GalleryMember; ratio: number }[] = [];
  const fitHeight = (values: typeof row) => Math.max(0, width - gap * (values.length - 1)) / values.reduce((sum, entry) => sum + entry.ratio, 0);
  const flush = (last: boolean) => {
    // A short final row stays left-aligned at the target size. A single very
    // wide image may need a smaller height to fit a narrow/mobile container.
    const height = Math.min(fitHeight(row), last ? targetHeight : maxHeight);
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
      if (previous.length && (fitHeight(row) < minHeight || (fitHeight(previous) <= maxHeight && Math.abs(fitHeight(previous) - targetHeight) < Math.abs(fitHeight(row) - targetHeight)))) {
        const carry = row.pop()!;
        flush(false);
        row = [carry];
      } else flush(false);
    }
  }
  if (row.length) flush(true);
  return { boxes, height: Math.max(0, top - gap) };
}
