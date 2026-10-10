import { describe, expect, it } from "vitest";

import { animatedPlaybackSelection, animationLockDelay, animationPlaybackWindow, lightboxNavigationState, visualMemberGroups } from "./GalleryDetailPage";
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

describe("lightboxNavigationState", () => {
  it("does not wrap at either end of the full gallery index", () => {
    expect(lightboxNavigationState(0, 3)).toEqual({ canPrevious: false, canNext: true });
    expect(lightboxNavigationState(1, 3)).toEqual({ canPrevious: true, canNext: true });
    expect(lightboxNavigationState(2, 3)).toEqual({ canPrevious: true, canNext: false });
  });
});

describe("animatedPlaybackSelection", () => {
  it("intersects the locked window with visible media and honours reduced motion", () => {
    const ordered = ["one", "two", "three", "four", "five", "six"];
	expect(animatedPlaybackSelection(ordered, new Set(["six", "five", "four", "three", "two"]), false)).toEqual(["two", "three", "four", "five", "six"]);
    expect(animatedPlaybackSelection(ordered, new Set(ordered), true)).toEqual([]);
  });
});

describe("animationPlaybackWindow", () => {
	const ordered = Array.from({ length: 20 }, (_, index) => `${index + 1}`);

	it("centres odd and even limits with the accepted right-side bias", () => {
		expect(animationPlaybackWindow(ordered, "10", 5)).toEqual(["8", "9", "10", "11", "12"]);
		expect(animationPlaybackWindow(ordered, "10", 6)).toEqual(["8", "9", "10", "11", "12", "13"]);
	});

	it("clamps anchors at the beginning and end and returns every item below the limit", () => {
		expect(animationPlaybackWindow(ordered, "2", 6)).toEqual(["1", "2", "3", "4", "5", "6"]);
		expect(animationPlaybackWindow(ordered, "19", 6)).toEqual(["15", "16", "17", "18", "19", "20"]);
		expect(animationPlaybackWindow(["1", "2"], "2", 12)).toEqual(["1", "2"]);
	});
});

describe("animationLockDelay", () => {
	it("requires 150 ms confirmation and extends it through the configured cooldown", () => {
		expect(animationLockDelay(10_000, 0, 800)).toBe(150);
		expect(animationLockDelay(10_000, 9_600, 800)).toBe(400);
		expect(animationLockDelay(10_000, 9_100, 800)).toBe(150);
	});
});
