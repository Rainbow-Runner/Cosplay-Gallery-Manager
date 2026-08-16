import { MockedProvider } from "@apollo/client/testing/react";
import { renderHook, waitFor } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { GalleryMember } from "./types";
import { originalImageURL, useImageDisplay } from "./useImageDisplay";

const cardResource = { itemUUID: "01900000-0000-7000-8000-000000000004", contentRevision: 3, profileHash: "card", variant: "CARD_480", mimeType: "image/jpeg" };
const item: GalleryMember = { itemUUID: cardResource.itemUUID, mediaKind: "STATIC_IMAGE", contentFormat: "IMAGE", position: "1024",
  caption: "", processingState: "READY", favorite: false, cardResource };

function wrapper({ children }: PropsWithChildren) { return <MockedProvider>{children}</MockedProvider>; }

describe("useImageDisplay", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("uses the opaque original route after the server accepts the static image", async () => {
    const fetch = vi.fn().mockResolvedValue(new Response(null, { status: 200 }));
    vi.stubGlobal("fetch", fetch);
    const { result } = renderHook(() => useImageDisplay(item), { wrapper });
    await waitFor(() => expect(result.current.url).toBe(`/resource/image/${item.itemUUID}/3/original`));
    expect(fetch).toHaveBeenCalledWith(originalImageURL(item), expect.objectContaining({ method: "HEAD", credentials: "same-origin" }));
    expect(result.current.mode).toBe("DIRECT");
  });

  it("plays an animated source through the same exact-original route", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(null, { status: 200 })));
    const animated = { ...item, mediaKind: "ANIMATED_IMAGE" as const };
    const { result } = renderHook(() => useImageDisplay(animated), { wrapper });
    await waitFor(() => expect(result.current.url).toBe(`/resource/image/${item.itemUUID}/3/original`));
    expect(result.current.failed).toBe(false);
  });
});
