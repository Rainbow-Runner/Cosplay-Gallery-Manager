import "@testing-library/jest-dom/vitest";

import type { MockedResponse } from "@apollo/client/testing";
import { MockedProvider } from "@apollo/client/testing/react";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, describe, expect, it } from "vitest";

import {
  BROWSE_UI_SETTINGS,
  GALLERY_DETAIL,
  GALLERY_MEMBER_INDEX,
  RECORD_GALLERY_VIEW,
  RELATED_GALLERIES,
  SET_BROWSE_GALLERY_COVER_ITEM,
  SET_ITEM_FAVORITE,
  SET_ITEM_RATING,
} from "../api/browse";
import { messages } from "../i18n/messages";
import { GalleryDetailPage } from "./GalleryDetailPage";
import type { BrowseGalleryCard, GalleryDetail, GalleryMember, GalleryMemberIndex, ResourceIdentity } from "./types";

afterEach(cleanup);

const resource = (itemUUID: string, mimeType = "image/jpeg"): ResourceIdentity => ({ itemUUID, contentRevision: 1, profileHash: "profile", variant: "CARD_480", mimeType });
const members: GalleryMember[] = [
  { itemUUID: "photo-1", mediaKind: "STATIC_IMAGE", contentFormat: "IMAGE", imageCategory: "PHOTO", position: "1024", caption: "First photo", processingState: "READY", cardResource: resource("photo-1"), largeResource: { ...resource("photo-1"), variant: "LIGHTBOX" }, favorite: false, ratingHalfSteps: null },
  { itemUUID: "photo-2", mediaKind: "STATIC_IMAGE", contentFormat: "IMAGE", imageCategory: "SELFIE", position: "2048", caption: "Second photo", processingState: "READY", cardResource: resource("photo-2"), largeResource: { ...resource("photo-2"), variant: "LIGHTBOX" }, favorite: false, ratingHalfSteps: null },
  { itemUUID: "gif-1", mediaKind: "ANIMATED_IMAGE", contentFormat: "IMAGE", imageCategory: null, position: "3072", caption: "Animated image", processingState: "READY", cardResource: resource("gif-1", "image/gif"), largeResource: null, favorite: false, ratingHalfSteps: null },
  { itemUUID: "video-1", mediaKind: "VIDEO", contentFormat: "VIDEO", imageCategory: null, position: "4096", caption: "Video", processingState: "READY", cardResource: resource("video-1", "video/mp4"), largeResource: null, favorite: false, ratingHalfSteps: null },
];

function card(coverItemUUID = "photo-1"): BrowseGalleryCard {
  return {
    __typename: "BrowseGalleryCard",
    setID: "gallery-1", slug: "gallery-one", title: "Gallery one", collectionType: "COSPLAY", contentRating: "NON_ADULT",
    cover: { kind: "ITEM", revision: 1, managed: false, warning: false, resource: resource(coverItemUUID) },
    credits: [{ uuid: "coser-1", name: "Alice" }], creditCount: 1,
    characters: [{ uuid: "character-1", name: "Saber" }], characterCount: 1,
    works: [{ uuid: "work-1", name: "Fate" }], workCount: 1,
    shootDate: "2026-07", shootDatePrecision: "MONTH", addedAtUTC: "2026-08-01T00:00:00Z",
    media: { photo: 1, selfie: 1, gif: 1, video: 1 }, favorite: false, ratingHalfSteps: null, scrubberCount: 0, scrubberRevision: 0,
  } as BrowseGalleryCard;
}

function detail(coverItemUUID = "photo-1"): GalleryDetail {
  return {
    card: card(coverItemUUID), description: "Description", photographerName: "Photographer", studioName: "Studio", availableBytes: 4096,
    mediaParentDirectories: ["/media/Gallery one", "/media/Gallery one/Disc 2"],
    credits: [{ coser: { uuid: "coser-1", name: "Alice" }, characters: [{ uuid: "character-1", name: "Saber" }], works: [{ uuid: "work-1", name: "Fate" }] }],
    tags: [{ uuid: "tag-1", name: "Outdoor" }], externalLinks: [], redirected: false,
  };
}

function memberIndex(metadataRevision = 7): GalleryMemberIndex {
  return { setID: "gallery-1", metadataRevision, scanRevision: 2, items: members };
}

function queryMocks(options: { refetchedCover?: string; initialEntry?: string } = {}): MockedResponse[] {
  const mocks: MockedResponse[] = [
    { request: { query: GALLERY_DETAIL, variables: { slug: "gallery-one" } }, result: { data: { galleryDetail: detail() } } },
    { request: { query: BROWSE_UI_SETTINGS }, result: { data: { browseUISettings: { settingsRevision: 1, galleryScrubberEnabled: true, detailMediaFilterEnabled: false, galleryAnimatedPlaybackLimit: 12, galleryAnimatedLockIntervalMS: 800, cardFavoriteControlVisible: true, cardRatingSummaryVisible: true, detailRatingControlVisible: true } } } },
    { request: { query: GALLERY_MEMBER_INDEX, variables: { setID: "gallery-1" } }, result: { data: { galleryMemberIndex: memberIndex() } } },
    { request: { query: RELATED_GALLERIES, variables: { setID: "gallery-1" } }, result: { data: { relatedGalleries: [] } } },
    { request: { query: RECORD_GALLERY_VIEW, variables: { setID: "gallery-1", itemUUID: null } }, result: { data: { recordGalleryView: true } } },
  ];
  if (options.initialEntry?.includes("item=")) mocks.push(
    { request: { query: RECORD_GALLERY_VIEW, variables: { setID: "gallery-1", itemUUID: "photo-2" } }, result: { data: { recordGalleryView: true } } },
  );
  if (options.refetchedCover) mocks.push(
    { request: { query: SET_BROWSE_GALLERY_COVER_ITEM, variables: { setID: "gallery-1", itemUUID: options.refetchedCover, expectedMetadataRevision: 7 } }, result: { data: { setGalleryCoverItem: { row: { metadataRevision: 8 } } } } },
    { request: { query: GALLERY_DETAIL, variables: { slug: "gallery-one" } }, result: { data: { galleryDetail: detail(options.refetchedCover) } } },
    { request: { query: GALLERY_MEMBER_INDEX, variables: { setID: "gallery-1" } }, result: { data: { galleryMemberIndex: memberIndex(8) } } },
  );
  return mocks;
}

function renderPage(mocks: MockedResponse[], initialEntry = "/gallery/gallery-one") {
  return render(
    <IntlProvider locale="en-GB" messages={messages["en-GB"]}>
      <MockedProvider mocks={mocks}>
        <MemoryRouter initialEntries={[initialEntry]}>
          <Routes><Route path="/gallery/:slug" element={<GalleryDetailPage />} /><Route path="/media/:uuid" element={<p>Media detail</p>} /></Routes>
        </MemoryRouter>
      </MockedProvider>
    </IntlProvider>,
  );
}

describe("GalleryDetailPage presentation and media actions", () => {
  it("renders ordered media groups without visible category headings", async () => {
    renderPage(queryMocks());
    expect(await screen.findByRole("heading", { name: "Gallery one" })).toBeInTheDocument();
    expect(document.querySelector(".gallery-hero")).not.toBeInTheDocument();
    expect(document.querySelectorAll(".media-tile")).toHaveLength(4);
    expect(screen.queryByRole("heading", { name: "Photos & selfies" })).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "GIF" })).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Video" })).not.toBeInTheDocument();
    expect(document.querySelectorAll(".media-tile__cover")).toHaveLength(1);
  });

  it("persists a media favourite from the tile control", async () => {
    const mocks = queryMocks();
    mocks.push({ request: { query: SET_ITEM_FAVORITE, variables: { itemUUID: "photo-1", favorite: true } }, result: { data: { setItemFavorite: { metadataRevision: 7 } } } });
    renderPage(mocks);
    const button = (await screen.findAllByRole("button", { name: "Favourite media" }))[0];
    fireEvent.click(button);
    await waitFor(() => expect(button).toHaveAttribute("aria-pressed", "true"));
  });

  it("sets a static item as cover with the current metadata revision", async () => {
    renderPage(queryMocks({ refetchedCover: "photo-2" }));
    const tile = (await screen.findByRole("button", { name: "Second photo" })).closest(".media-tile");
    expect(tile).not.toBeNull();
    fireEvent.click(within(tile as HTMLElement).getByLabelText("Media actions"));
    fireEvent.click(within(tile as HTMLElement).getByRole("button", { name: "Set as cover" }));
    expect(await screen.findByText("Cover updated")).toBeInTheDocument();
    await waitFor(() => expect(screen.getByRole("button", { name: "Second photo" }).closest(".media-tile")?.querySelector(".media-tile__cover")).not.toBeNull());
  });

  it("closes an open media actions menu when the user clicks outside it", async () => {
    renderPage(queryMocks());
    const tile = (await screen.findByRole("button", { name: "Second photo" })).closest(".media-tile") as HTMLElement;
    const menu = tile.querySelector("details.media-tile__menu") as HTMLDetailsElement;
    const trigger = within(tile).getByLabelText("Media actions");
    expect(trigger).not.toHaveAttribute("title");
    fireEvent.click(trigger);
    await waitFor(() => expect(menu).toHaveAttribute("open"));
    fireEvent.pointerDown(screen.getByRole("heading", { name: "Gallery one" }));
    await waitFor(() => expect(menu).not.toHaveAttribute("open"));
  });

  it("shows local media folders and closes More details outside or with Escape", async () => {
    renderPage(queryMocks());
    const trigger = await screen.findByText("More details");
    const details = trigger.closest("details") as HTMLDetailsElement;
    fireEvent.click(trigger);
    await waitFor(() => expect(details).toHaveAttribute("open"));
    expect(screen.getByText("/media/Gallery one")).toBeInTheDocument();
    expect(screen.getByText("/media/Gallery one/Disc 2")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Manage this gallery" })).toHaveAttribute("href", "/manage/gallery/gallery-1?tab=media");
    fireEvent.pointerDown(screen.getByText("/media/Gallery one"));
    expect(details).toHaveAttribute("open");
    fireEvent.keyDown(document, { key: "Escape" });
    await waitFor(() => expect(details).not.toHaveAttribute("open"));
    fireEvent.click(trigger);
    await waitFor(() => expect(details).toHaveAttribute("open"));
    fireEvent.pointerDown(screen.getByRole("heading", { name: "Gallery one" }));
    await waitFor(() => expect(details).not.toHaveAttribute("open"));
  });

  it("opens a deep-linked item and removes the dialog when it is closed", async () => {
    const initialEntry = "/gallery/gallery-one?item=photo-2";
    renderPage(queryMocks({ initialEntry }), initialEntry);
    const dialog = await screen.findByRole("dialog", { name: "Second photo" });
    expect(within(dialog).getByRole("button", { name: "Favourite media" })).toHaveAttribute("title", "Favourite media");
    expect(within(dialog).getByRole("combobox", { name: "Media rating" })).toHaveAttribute("title", "Media rating");
    expect(within(dialog).getByRole("button", { name: "Set as cover" })).toHaveAttribute("title", "Set as cover");
    expect(within(dialog).getByRole("link", { name: "Open media details" })).toHaveAttribute("title", "Open media details");
    expect(within(dialog).getByRole("button", { name: "Close media viewer" })).toHaveAttribute("title", "Close media viewer");
    fireEvent.click(screen.getByRole("button", { name: "Close media viewer" }));
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Second photo" })).not.toBeInTheDocument());
  });

  it("persists a Lightbox half-star rating with the current metadata revision", async () => {
    const initialEntry = "/gallery/gallery-one?item=photo-2";
    const mocks = queryMocks({ initialEntry });
    mocks.push({ request: { query: SET_ITEM_RATING, variables: { itemUUID: "photo-2", ratingHalfSteps: 8, expectedMetadataRevision: 7 } }, result: { data: { setItemRating: { metadataRevision: 8 } } } });
    renderPage(mocks, initialEntry);
    const rating = await screen.findByLabelText("Media rating");
    fireEvent.change(rating, { target: { value: "8" } });
    await waitFor(() => expect(rating).toHaveValue("8"));
  });
});
