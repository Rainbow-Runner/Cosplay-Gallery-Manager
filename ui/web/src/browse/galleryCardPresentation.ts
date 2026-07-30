import type { BrowseGalleryCard } from "./types";

function uniqueNames(values: { name: string }[]) {
  return [...new Set(values.map((value) => value.name.trim()).filter(Boolean))];
}

export function galleryCardPresentation(card: BrowseGalleryCard) {
  const characters = uniqueNames(card.characters);
  const works = uniqueNames(card.works);
  const cosers = uniqueNames(card.credits);
  return {
    primary: card.collectionType === "COSPLAY" && characters.length ? characters.join(" · ") : card.title,
    secondary: works.join(" · ") || (card.collectionType === "ALBUM" ? "Album" : ""),
    cosers,
  };
}
