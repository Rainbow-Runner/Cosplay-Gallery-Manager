import { describe, expect, it } from "vitest";

import { formatGalleryMediaCount } from "./galleryMediaCount";

describe("formatGalleryMediaCount", () => {
  it("omits media types whose count is zero", () => {
    expect(formatGalleryMediaCount({ photo: 100, selfie: 0, gif: 0, video: 0 })).toBe("100P");
  });

  it("keeps non-zero media types in the established display order", () => {
    expect(formatGalleryMediaCount({ photo: 96, selfie: 4, gif: 2, video: 1 })).toBe("96P 4S 2G 1V");
  });

  it("returns an empty label when every media count is zero", () => {
    expect(formatGalleryMediaCount({ photo: 0, selfie: 0, gif: 0, video: 0 })).toBe("");
  });
});
