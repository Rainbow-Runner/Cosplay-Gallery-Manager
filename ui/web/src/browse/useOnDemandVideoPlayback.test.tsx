import type { MockedResponse } from "@apollo/client/testing";
import { MockedProvider } from "@apollo/client/testing/react";
import { renderHook, waitFor } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { describe, expect, it } from "vitest";

import { ITEM_VIDEO_PLAYBACK_STATUS, REQUEST_ITEM_VIDEO_PLAYBACK } from "../api/browse";
import type { GalleryMember, ResourceIdentity, VideoPlaybackStatus } from "./types";
import { useOnDemandVideoPlayback } from "./useOnDemandVideoPlayback";

const item: GalleryMember = { itemUUID: "01900000-0000-7000-8000-000000000002", mediaKind: "VIDEO", contentFormat: "VIDEO", position: "1024",
  caption: "", processingState: "READY", favorite: false };

function wrapper(mocks: MockedResponse[] = []) {
  return function ApolloWrapper({ children }: PropsWithChildren) { return <MockedProvider mocks={mocks}>{children}</MockedProvider>; };
}

function mocksFor(response: VideoPlaybackStatus): MockedResponse[] {
  return [
    { request: { query: ITEM_VIDEO_PLAYBACK_STATUS, variables: { itemUUID: item.itemUUID } }, result: { data: { itemVideoPlaybackStatus: response } } },
    { request: { query: REQUEST_ITEM_VIDEO_PLAYBACK, variables: { itemUUID: item.itemUUID } }, result: { data: { requestItemVideoPlayback: response } } },
  ];
}

describe("useOnDemandVideoPlayback", () => {
  it("builds the opaque direct route without a derivative identity", async () => {
    const response: VideoPlaybackStatus = { itemUUID: item.itemUUID, mode: "DIRECT", status: "READY", contentRevision: 4, resource: null, errorCode: "" };
    const { result } = renderHook(() => useOnDemandVideoPlayback(item), { wrapper: wrapper(mocksFor(response)) });
    await waitFor(() => expect(result.current.url).toBe(`/resource/video/${item.itemUUID}/4/direct`));
    expect(result.current.preparing).toBe(false);
  });

  it("uses the authenticated derivative route for a ready proxy", async () => {
    const resource: ResourceIdentity = { itemUUID: item.itemUUID, contentRevision: 4, profileHash: "playback-profile", variant: "VIDEO_PLAYBACK", mimeType: "video/mp4" };
    const response: VideoPlaybackStatus = { itemUUID: item.itemUUID, mode: "REMUX", status: "READY", contentRevision: 4, resource, errorCode: "" };
    const { result } = renderHook(() => useOnDemandVideoPlayback(item), { wrapper: wrapper(mocksFor(response)) });
    await waitFor(() => expect(result.current.url).toContain("/resource/item/"));
    expect(result.current.url).toContain("VIDEO_PLAYBACK");
  });

  it("does not request playback for a gallery image", () => {
    const { result } = renderHook(() => useOnDemandVideoPlayback({ ...item, mediaKind: "STATIC_IMAGE", contentFormat: "IMAGE" }), { wrapper: wrapper() });
    expect(result.current.preparing).toBe(false);
    expect(result.current.url).toBeNull();
  });
});
