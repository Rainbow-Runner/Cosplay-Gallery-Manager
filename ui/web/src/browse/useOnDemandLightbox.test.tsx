import type { MockedResponse } from "@apollo/client/testing";
import { MockedProvider } from "@apollo/client/testing/react";
import { renderHook, waitFor } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { describe, expect, it } from "vitest";

import { ITEM_LIGHTBOX_STATUS, REQUEST_ITEM_LIGHTBOX } from "../api/browse";
import type { GalleryMember, ResourceIdentity } from "./types";
import { useOnDemandLightbox } from "./useOnDemandLightbox";

const card: ResourceIdentity = { itemUUID: "01900000-0000-7000-8000-000000000001", contentRevision: 1, profileHash: "profile", variant: "CARD_480", mimeType: "image/jpeg" };
const large: ResourceIdentity = { ...card, variant: "LIGHTBOX_4096" };
const item: GalleryMember = { itemUUID: card.itemUUID, mediaKind: "STATIC_IMAGE", contentFormat: "IMAGE", imageCategory: "PHOTO", position: "1024", caption: "", processingState: "READY", cardResource: card, favorite: false };

function wrapper(mocks: MockedResponse[] = []) {
  return function ApolloWrapper({ children }: PropsWithChildren) { return <MockedProvider mocks={mocks}>{children}</MockedProvider>; };
}

describe("useOnDemandLightbox", () => {
  it("uses an existing large resource without requesting generation", () => {
    const { result } = renderHook(() => useOnDemandLightbox({ ...item, largeResource: large }), { wrapper: wrapper() });
    expect(result.current).toEqual({ resource: large, preparing: false, failed: false });
  });

  it("requests a missing large resource and settles on its opaque identity", async () => {
    const response = { status: "READY", errorCode: "", resource: large };
    const mocks: MockedResponse[] = [
      { request: { query: ITEM_LIGHTBOX_STATUS, variables: { itemUUID: item.itemUUID } }, result: { data: { itemLightboxStatus: response } } },
      { request: { query: REQUEST_ITEM_LIGHTBOX, variables: { itemUUID: item.itemUUID } }, result: { data: { requestItemLightbox: response } } },
    ];
    const { result } = renderHook(() => useOnDemandLightbox(item), { wrapper: wrapper(mocks) });
    await waitFor(() => expect(result.current.resource).toEqual(large));
    expect(result.current.preparing).toBe(false);
    expect(result.current.failed).toBe(false);
  });
});
