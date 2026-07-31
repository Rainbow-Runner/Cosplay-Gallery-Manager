import type { BrowseGalleryCard } from "./types";

function uniqueNames(values: { name: string }[]) {
  return [...new Set(values.map((value) => value.name.trim()).filter(Boolean))];
}

function uniqueEntities(values: BrowseGalleryCard["credits"]) {
  const seen = new Set<string>();
  return values.flatMap((value) => {
    const name = value.name.trim();
    if (!name || seen.has(value.uuid)) return [];
    seen.add(value.uuid);
    return [{ ...value, name }];
  });
}

export function galleryCardPresentation(card: BrowseGalleryCard) {
  const characters = uniqueNames(card.characters);
  const works = uniqueNames(card.works);
  const cosers = uniqueEntities(card.credits);
  return {
    primary: card.collectionType === "COSPLAY" && characters.length ? characters.join(" · ") : card.title,
    secondary: works.join(" · ") || (card.collectionType === "ALBUM" ? "Album" : ""),
    cosers,
  };
}
