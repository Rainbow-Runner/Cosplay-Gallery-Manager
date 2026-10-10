import { describe, expect, it } from "vitest";
import type { BrowseGalleryCard } from "./types";
import { galleryCardPresentation } from "./galleryCardPresentation";

const base = {
  title: "Saber at the lake", collectionType: "COSPLAY", characters: [{ uuid: "c", name: "Saber" }],
  works: [{ uuid: "w", name: "Fate/stay night" }], credits: [{ uuid: "p", name: "Alice" }],
} as BrowseGalleryCard;

describe("galleryCardPresentation", () => {
  it("uses Character, Work and Coser hierarchy for cosplay", () => {
    expect(galleryCardPresentation(base)).toEqual({
      primary: "Saber",
      secondary: "Fate/stay night",
      cosers: [{ uuid: "p", name: "Alice" }],
    });
  });
  it("uses the Gallery title and Album fallback for albums", () => {
    expect(galleryCardPresentation({ ...base, collectionType: "ALBUM", characters: [], works: [] }))
      .toEqual({ primary: "Saber at the lake", secondary: "Album", cosers: [{ uuid: "p", name: "Alice" }] });
  });
  it("deduplicates repeated entity identities without collapsing different cosers with the same name", () => {
    expect(galleryCardPresentation({
      ...base,
      credits: [...base.credits, ...base.credits, { uuid: "p2", name: "Alice" }],
    }).cosers).toEqual([{ uuid: "p", name: "Alice" }, { uuid: "p2", name: "Alice" }]);
  });
});
