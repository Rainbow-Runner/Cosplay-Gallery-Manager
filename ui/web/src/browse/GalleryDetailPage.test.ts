import { describe, expect, it } from "vitest";

import { visualMemberGroups } from "./GalleryDetailPage";
import type { GalleryMember } from "./types";

function member(itemUUID: string, mediaKind: GalleryMember["mediaKind"], imageCategory?: GalleryMember["imageCategory"]): GalleryMember {
  return { itemUUID, mediaKind, imageCategory, contentFormat: mediaKind === "VIDEO" ? "VIDEO" : "IMAGE", position: "1024",
    caption: "", processingState: "READY", favorite: false };
}

describe("visualMemberGroups", () => {
  const items = [member("photo", "STATIC_IMAGE", "PHOTO"), member("selfie", "STATIC_IMAGE", "SELFIE"),
    member("gif", "ANIMATED_IMAGE"), member("video", "VIDEO")];

  it("keeps Photo and Selfie in one visual group while preserving backend order", () => {
    const groups = visualMemberGroups(items, "ALL");
    expect(groups.map((group) => group.key)).toEqual(["photo", "gif", "video"]);
    expect(groups[0].items.map((item) => item.itemUUID)).toEqual(["photo", "selfie"]);
  });

  it("supports the optional category filter without changing the full index", () => {
    expect(visualMemberGroups(items, "SELFIE").flatMap((group) => group.items).map((item) => item.itemUUID)).toEqual(["selfie"]);
  });
});
