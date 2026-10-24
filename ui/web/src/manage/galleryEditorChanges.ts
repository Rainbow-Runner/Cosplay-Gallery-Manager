import type { ManageGalleryDetail } from "./types";

export function galleryMetadataInput(value: ManageGalleryDetail) {
  return {
    title: value.row.title,
    ...(value.followSourceTitle === undefined ? {} : { followSourceTitle: value.followSourceTitle }),
    aliases: value.aliases.map(alias => alias.trim().normalize("NFC")),
    description: value.description,
    shootDate: value.shootDate, shootDatePrecision: value.shootDate ? value.shootDatePrecision : "UNKNOWN",
    publishDate: value.publishDate, publishDatePrecision: value.publishDate ? value.publishDatePrecision : "UNKNOWN",
    contentRating: value.row.contentRating || "",
    photographerName: value.photographerName, studioName: value.studioName,
  };
}

export function galleryMetadataKey(value: ManageGalleryDetail) {
  return JSON.stringify(galleryMetadataInput({ ...value, followSourceTitle: value.followSourceTitle === true }));
}

export function galleryRelationsInput(value: ManageGalleryDetail) {
  return {
    credits: value.credits.map((credit, index) => ({
      coserUUID: credit.coserUUID, position: String((index + 1) * 1024),
      cast: credit.cast.map((cast, index) => ({ characterUUID: cast.characterUUID, position: String((index + 1) * 1024) })),
    })),
    tags: value.tags.map((tag, index) => ({ tagUUID: tag.uuid, position: String((index + 1) * 1024) })),
  };
}

export function galleryRelationsKey(value: ManageGalleryDetail) {
  // Names, Work labels, Apollo fields and stored numeric position gaps are
  // presentation data; UUIDs, ownership and the visible order are meaningful.
  return JSON.stringify(galleryRelationsInput(value));
}
