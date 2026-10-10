import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { GalleryCardPosterScrubber } from "./GalleryCardPosterScrubber";

describe("GalleryCardPosterScrubber", () => {
  beforeEach(() => {
    vi.stubGlobal("matchMedia", vi.fn(() => ({ matches: true, addEventListener: vi.fn(), removeEventListener: vi.fn() })));
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => { callback(0); return 1; });
    vi.stubGlobal("cancelAnimationFrame", vi.fn());
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ ok: true, blob: async () => new Blob(["poster"], { type: "image/jpeg" }) }));
    Object.defineProperty(URL, "createObjectURL", { configurable: true, value: vi.fn(() => "blob:preview") });
    Object.defineProperty(URL, "revokeObjectURL", { configurable: true, value: vi.fn() });
  });

  afterEach(() => vi.unstubAllGlobals());

  it("maps horizontal position to ordinal and restores the cover on leave", async () => {
    const { container, unmount } = render(
      <GalleryCardPosterScrubber coverURL="/cover.jpg" previewCount={4} previewURL={(ordinal) => `/preview/${ordinal}`} enabled alt="Gallery" />,
    );
    const scrubber = container.querySelector<HTMLElement>(".poster-scrubber");
    expect(scrubber).not.toBeNull();
    vi.spyOn(scrubber!, "getBoundingClientRect").mockReturnValue({ x: 0, y: 0, left: 0, top: 0, right: 100, bottom: 133, width: 100, height: 133, toJSON: () => ({}) });
    fireEvent.pointerMove(scrubber!, { pointerType: "mouse", clientX: 75 });
    await waitFor(() => expect(fetch).toHaveBeenCalledWith("/preview/3", expect.objectContaining({ credentials: "same-origin" })));
    await waitFor(() => expect(screen.getByRole("img", { name: "Gallery" })).toHaveAttribute("src", "blob:preview"));
    fireEvent.pointerLeave(scrubber!);
    expect(screen.getByRole("img", { name: "Gallery" }).getAttribute("src")).toContain("cover.jpg");
    unmount();
    expect(URL.revokeObjectURL).toHaveBeenCalledWith("blob:preview");
  });

  it("does not request previews when disabled", () => {
    const { container } = render(
      <GalleryCardPosterScrubber coverURL="/cover.jpg" previewCount={4} previewURL={(ordinal) => `/preview/${ordinal}`} enabled={false} alt="Gallery" />,
    );
    const scrubber = container.querySelector<HTMLElement>(".poster-scrubber")!;
    vi.spyOn(scrubber, "getBoundingClientRect").mockReturnValue({ x: 0, y: 0, left: 0, top: 0, right: 100, bottom: 133, width: 100, height: 133, toJSON: () => ({}) });
    fireEvent.pointerMove(scrubber, { pointerType: "mouse", clientX: 75 });
    expect(fetch).not.toHaveBeenCalled();
  });
});
