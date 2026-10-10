import { describe, expect, it } from "vitest";
import { estimateTimelineBatch, groupTimelineMonths } from "./CoserMonthTimeline";
import type { BrowseGalleryCard } from "./types";

describe("monthly timeline batches", () => {
  it("merges a month spanning batches and removes repeated cards", () => {
    const card = (setID: string) => ({ setID } as BrowseGalleryCard);
    const result = groupTimelineMonths([
      { month: "2026-10", card: card("new") },
      { month: "2026-10", card: card("next") },
      { month: "2026-10", card: card("next") },
      { month: "2026-09", card: card("old") },
    ]);
    expect(result.map((group) => [group.month, group.cards.map((item) => item.setID)])).toEqual([
      ["2026-10", ["new", "next"]], ["2026-09", ["old"]],
    ]);
  });
  it("estimates enough cards for the viewport with bounded requests", () => {
    expect(estimateTimelineBatch(320, 600, 390)).toBeGreaterThanOrEqual(6);
    expect(estimateTimelineBatch(2400, 1000, 2560)).toBe(15);
    expect(estimateTimelineBatch(1, 100000, 1920)).toBe(24);
    expect(estimateTimelineBatch(320, 0, 390)).toBe(4);
  });
});
