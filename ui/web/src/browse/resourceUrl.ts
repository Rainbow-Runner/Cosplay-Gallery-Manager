import type { BrowseGalleryCard, ResourceIdentity } from "./types";

export function itemResourceURL(resource?: ResourceIdentity | null): string | null {
  if (!resource) return null;
  return `/resource/item/${encodeURIComponent(resource.itemUUID)}/${resource.contentRevision}/${encodeURIComponent(resource.profileHash)}/${encodeURIComponent(resource.variant)}`;
}

export function galleryPreviewURL(card: BrowseGalleryCard, ordinal: number): string {
  return `/resource/gallery-preview/${encodeURIComponent(card.setID)}/${card.scrubberRevision}/${ordinal}`;
}
