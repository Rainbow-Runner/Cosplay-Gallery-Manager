import type { ManageGalleryItem } from "./types";

export const mediaGroupOrder = ["PHOTO", "SELFIE", "ANIMATED_IMAGE", "VIDEO"] as const;
export type GalleryMediaGroupKey = typeof mediaGroupOrder[number];

export interface GalleryMediaFolder {
  path: string;
  items: ManageGalleryItem[];
}

export interface GalleryMediaGroup {
  key: GalleryMediaGroupKey;
  label: string;
  folders: GalleryMediaFolder[];
}

export function galleryMediaGroupKey(item: ManageGalleryItem): GalleryMediaGroupKey {
  if (item.mediaKind === "STATIC_IMAGE") return item.imageCategory === "SELFIE" ? "SELFIE" : "PHOTO";
  return item.mediaKind === "ANIMATED_IMAGE" ? "ANIMATED_IMAGE" : "VIDEO";
}

export function galleryMediaParentPath(relativePath: string) {
  const separator = relativePath.lastIndexOf("/");
  return separator < 0 ? "" : relativePath.slice(0, separator);
}

export function galleryMediaFileName(relativePath: string) {
  const separator = relativePath.lastIndexOf("/");
  return separator < 0 ? relativePath : relativePath.slice(separator + 1);
}

export function groupGalleryMedia(items: ManageGalleryItem[]): GalleryMediaGroup[] {
  return mediaGroupOrder.flatMap((key) => {
    const groupItems = items.filter((item) => galleryMediaGroupKey(item) === key);
    if (!groupItems.length) return [];
    const byFolder = new Map<string, ManageGalleryItem[]>();
    for (const item of groupItems) {
      const folder = galleryMediaParentPath(item.relativePath);
      byFolder.set(folder, [...(byFolder.get(folder) || []), item]);
    }
    const folderPaths = [...byFolder.keys()].filter(Boolean);
    const orderedPaths = byFolder.has("") ? ["", ...folderPaths] : folderPaths;
    return [{
      key,
      label: key === "PHOTO" ? "Photos" : key === "SELFIE" ? "Selfies" : key === "ANIMATED_IMAGE" ? "GIFs" : "Videos",
      folders: orderedPaths.map((path) => ({ path, items: byFolder.get(path) || [] })),
    }];
  });
}

export function flattenGalleryMediaFolders(folders: GalleryMediaFolder[]) {
  return folders.flatMap((folder) => folder.items);
}

export function naturalFileNameCompare(left: ManageGalleryItem, right: ManageGalleryItem) {
  const compared = naturalTextCompare(galleryMediaFileName(left.relativePath), galleryMediaFileName(right.relativePath));
  return compared || naturalTextCompare(left.relativePath, right.relativePath) || left.uuid.localeCompare(right.uuid);
}

function naturalTextCompare(left: string, right: string) {
  const l = Array.from(left.toLowerCase());
  const r = Array.from(right.toLowerCase());
  let li = 0;
  let ri = 0;
  while (li < l.length && ri < r.length) {
    const leftDigit = isDigit(l[li]);
    const rightDigit = isDigit(r[ri]);
    if (leftDigit && rightDigit) {
      let lnext = li;
      let rnext = ri;
      while (lnext < l.length && isDigit(l[lnext])) lnext++;
      while (rnext < r.length && isDigit(r[rnext])) rnext++;
      const lraw = l.slice(li, lnext).join("");
      const rraw = r.slice(ri, rnext).join("");
      const lnumber = lraw.replace(/^0+/, "") || "0";
      const rnumber = rraw.replace(/^0+/, "") || "0";
      if (lnumber.length !== rnumber.length) return lnumber.length - rnumber.length;
      if (lnumber !== rnumber) return lnumber < rnumber ? -1 : 1;
      if (lraw.length !== rraw.length) return lraw.length - rraw.length;
      li = lnext;
      ri = rnext;
      continue;
    }
    if (l[li] !== r[ri]) return (l[li].codePointAt(0) || 0) - (r[ri].codePointAt(0) || 0);
    li++;
    ri++;
  }
  if (li !== l.length || ri !== r.length) return li === l.length ? -1 : 1;
  return left < right ? -1 : left > right ? 1 : 0;
}

function isDigit(value: string) {
  return /^\p{Nd}$/u.test(value);
}
