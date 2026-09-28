import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { galleryMediaLayoutStorageKey, justifiedMediaLayout, readGalleryMediaLayout, saveGalleryMediaLayout } from "./galleryMediaLayout";
import type { GalleryMember } from "./types";

function item(uuid: string, width = 480, height = 320): GalleryMember {
  return { itemUUID: uuid, previewWidth: width, previewHeight: height, mediaKind: "STATIC_IMAGE", contentFormat: "IMAGE", position: uuid, caption: "", processingState: "READY", favorite: false };
}
beforeEach(() => {
  const values = new Map<string, string>();
  vi.stubGlobal("localStorage", { getItem: (key: string) => values.get(key) ?? null, setItem: (key: string, value: string) => values.set(key, value), clear: () => values.clear() });
});
afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); });

describe("justifiedMediaLayout", () => {
  it("preserves mixed media order, equal row heights, spacing and bounds", () => {
    const items = Array.from({ length: 25 }, (_, index) => item(String(index), index % 3 === 0 ? 480 : 320, index % 3 === 0 ? 320 : 480));
    const { boxes, height } = justifiedMediaLayout(items, 980);
    expect([...boxes.keys()]).toEqual(items.map((value) => value.itemUUID));
    let previousTop = -1, previousRight = -12, previousHeight = 0;
    for (const box of boxes.values()) {
      expect(box.width).toBeGreaterThan(0);
      expect(box.width).toBeLessThanOrEqual(480);
      expect(box.height).toBeLessThanOrEqual(300);
      expect(box.left + box.width).toBeLessThanOrEqual(980.001);
      if (box.top === previousTop) {
        expect(box.height).toBe(previousHeight);
        expect(box.left).toBeCloseTo(previousRight + 12);
      } else {
        expect(box.top).toBeGreaterThan(previousTop);
        expect(box.left).toBe(0);
      }
      previousTop = box.top; previousRight = box.left + box.width; previousHeight = box.height;
    }
    expect(height).toBeCloseTo(previousTop + previousHeight);
  });

  it("does not stretch the last row and handles pending or extreme proportions", () => {
    const last = justifiedMediaLayout([item("one")], 1400).boxes.get("one")!;
    expect(last.left).toBe(0); expect(last.height).toBe(240); expect(last.width).toBe(360);
    const { boxes } = justifiedMediaLayout([item("pending", 0, 0), item("wide", 100000, 1), item("tall", 1, 100000)], 720);
    expect(boxes.get("pending")!.width / boxes.get("pending")!.height).toBeCloseTo(0.75);
    expect(boxes.get("wide")!.width / boxes.get("wide")!.height).toBeCloseTo(2.4);
    expect(boxes.get("tall")!.width / boxes.get("tall")!.height).toBeCloseTo(0.35);
  });

  it("reflows responsively without reordering and ignores invalid container widths", () => {
    const items = Array.from({ length: 20 }, (_, i) => item(String(i)));
    const narrow = justifiedMediaLayout(items, 320), wide = justifiedMediaLayout(items, 1200);
    expect(narrow.height).toBeGreaterThan(wide.height);
    expect([...narrow.boxes.keys()]).toEqual([...wide.boxes.keys()]);
    expect(justifiedMediaLayout(items, 0).boxes.size).toBe(0);
    expect(justifiedMediaLayout(items, NaN).height).toBe(0);
  });
});

describe("layout preference", () => {
  it("defaults to grid, persists justified, and tolerates disabled storage", () => {
    expect(readGalleryMediaLayout()).toBe("GRID");
    saveGalleryMediaLayout("JUSTIFIED"); expect(readGalleryMediaLayout()).toBe("JUSTIFIED");
    window.localStorage.setItem(galleryMediaLayoutStorageKey, "unknown"); expect(readGalleryMediaLayout()).toBe("GRID");
    vi.spyOn(window.localStorage, "getItem").mockImplementation(() => { throw new Error("blocked"); });
    vi.spyOn(window.localStorage, "setItem").mockImplementation(() => { throw new Error("blocked"); });
    expect(readGalleryMediaLayout()).toBe("GRID");
    expect(() => saveGalleryMediaLayout("JUSTIFIED")).not.toThrow();
  });
});
