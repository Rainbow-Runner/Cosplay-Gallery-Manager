import "@testing-library/jest-dom/vitest";

import type { MockedResponse } from "@apollo/client/testing";
import { MockedProvider } from "@apollo/client/testing/react";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  BROWSE_TAG_OPTIONS,
  BROWSE_UI_SETTINGS,
  GALLERY_DETAIL,
  GALLERY_MEMBER_INDEX,
  RECORD_GALLERY_VIEW,
  REPLACE_GALLERY_TAGS,
  RELATED_GALLERIES,
  SET_BROWSE_GALLERY_COVER_ITEM,
  SET_ITEM_FAVORITE,
  SET_ITEM_RATING,
} from "../api/browse";
import { messages } from "../i18n/messages";
import { GalleryDetailPage } from "./GalleryDetailPage";
import type { BrowseGalleryCard, GalleryDetail, GalleryMember, GalleryMemberIndex, ResourceIdentity } from "./types";

beforeEach(() => {
  const values = new Map<string, string>();
  vi.stubGlobal("localStorage", { getItem: (key: string) => values.get(key) ?? null, setItem: (key: string, value: string) => values.set(key, value), clear: () => values.clear() });
});
afterEach(() => { cleanup(); vi.restoreAllMocks(); vi.unstubAllGlobals(); });

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
    credits: [{ uuid: "coser-1", name: "Alice", avatarURL: null }], creditCount: 1,
    characters: [{ uuid: "character-1", name: "Saber" }], characterCount: 1,
    works: [{ uuid: "work-1", name: "Fate" }], workCount: 1,
    shootDate: "2026-07", shootDatePrecision: "MONTH", publishDate: "", publishDatePrecision: "UNKNOWN", addedAtUTC: "2026-08-01T00:00:00Z",
    mediaAddedStartUTC: "", mediaAddedEndUTC: "", mediaAddedStatus: "NONE",
    media: { photo: 1, selfie: 1, gif: 1, video: 1 }, favorite: false, ratingHalfSteps: null, scrubberCount: 0, scrubberRevision: 0,
  } as BrowseGalleryCard;
}

function detail(coverItemUUID = "photo-1"): GalleryDetail {
  return {
    card: card(coverItemUUID), metadataRevision: 7, description: "Description", photographerName: "Photographer", studioName: "Studio", availableBytes: 4096,
    imageCaptureStart: "", imageCaptureEnd: "", videoCaptureStart: "", videoCaptureEnd: "",
    mediaParentDirectories: ["/media/Gallery one", "/media/Gallery one/Disc 2"],
    credits: [{ coser: { uuid: "coser-1", name: "Alice", avatarURL: null }, characters: [{ uuid: "character-1", name: "Saber" }], works: [{ uuid: "work-1", name: "Fate" }] }],
    tags: [{ uuid: "tag-1", name: "Outdoor" }], externalLinks: [], redirected: false,
  };
}

function memberIndex(metadataRevision = 7): GalleryMemberIndex {
  return { setID: "gallery-1", metadataRevision, scanRevision: 2, items: members.map((item, index) => ({ ...item, previewWidth: index === 0 ? 480 : 320, previewHeight: index === 0 ? 320 : 480 })) };
}

function queryMocks(options: { refetchedCover?: string; initialEntry?: string; filtersEnabled?: boolean } = {}): MockedResponse[] {
  const mocks: MockedResponse[] = [
    { request: { query: GALLERY_DETAIL, variables: { slug: "gallery-one" } }, result: { data: { galleryDetail: detail() } } },
    { request: { query: BROWSE_UI_SETTINGS }, result: { data: { browseUISettings: { settingsRevision: 1, galleryScrubberEnabled: true, detailMediaFilterEnabled: options.filtersEnabled ?? false, galleryAnimatedPlaybackLimit: 12, galleryAnimatedLockIntervalMS: 800, cardFavoriteControlVisible: true, cardRatingSummaryVisible: true, detailRatingControlVisible: true } } } },
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
  it("places compact accessible layout controls immediately before the header favourite, without a separate media toolbar", async () => {
    renderPage(queryMocks());
    const layout = await screen.findByRole("group", { name: "Media layout" });
    const favourite = await screen.findByRole("button", { name: "Favourite gallery" });
    expect(layout.parentElement).toHaveClass("gallery-detail__actions");
    expect(layout.nextElementSibling).toBe(favourite);
    for (const name of ["Card grid", "Justified rows"]) {
      const button = within(layout).getByRole("button", { name });
      expect(button).toHaveAttribute("title", name);
      expect(button.textContent).toBe("");
    }
    expect(document.querySelector(".gallery-media-toolbar")).not.toBeInTheDocument();
    expect(document.querySelector(".gallery-members .media-layout-switch")).not.toBeInTheDocument();
  });

  it("switches layouts without remounting media, changing order or closing an open menu; remembers the choice", async () => {
    renderPage(queryMocks());
    await waitFor(() => expect(document.querySelectorAll(".media-tile")).toHaveLength(4));
    const originalTiles = [...document.querySelectorAll(".media-tile")];
    const originalImages = [...document.querySelectorAll(".media-tile__open img")];
    const firstMenu = originalTiles[0].querySelector("details")!;
    fireEvent.click(within(originalTiles[0] as HTMLElement).getByLabelText("Media actions"));
    firstMenu.open = true;
    fireEvent.click(screen.getByRole("button", { name: "Justified rows" }));
    expect(screen.getByRole("button", { name: "Justified rows" })).toHaveAttribute("aria-pressed", "true");
    expect(document.querySelector(".media-sequence")).toHaveAttribute("data-layout", "JUSTIFIED");
    expect([...document.querySelectorAll(".media-tile")]).toEqual(originalTiles);
    originalImages.forEach((image, index) => expect(document.querySelectorAll(".media-tile__open img")[index]).toBe(image));
    expect((originalTiles[0] as HTMLElement).style.position).toBe("absolute");
    expect(firstMenu.open).toBe(true);
    expect(window.localStorage.getItem("cgm.gallery.media-layout")).toBe("JUSTIFIED");
    fireEvent.click(screen.getByRole("button", { name: "Card grid" }));
    expect((originalTiles[0] as HTMLElement).style.position).toBe("");
    expect(document.querySelector(".media-sequence")).toHaveAttribute("data-layout", "GRID");
  });

  it("keeps optional media filtering below the header independently of layout controls", async () => {
    renderPage(queryMocks({ filtersEnabled: true }));
    const filter = await screen.findByRole("group", { name: "Media filter" });
    expect(filter.parentElement).toHaveClass("gallery-members");
    fireEvent.click(within(filter).getByRole("button", { name: "VIDEO" }));
    await waitFor(() => expect(document.querySelectorAll(".media-tile")).toHaveLength(1));
    expect(document.querySelector(".media-tile")).toHaveAttribute("data-item-uuid", "video-1");
    expect(screen.getByRole("button", { name: "Card grid" })).toHaveAttribute("aria-pressed", "true");
  });

  it("restores justified preference and keeps lightbox navigation in business order", async () => {
    window.localStorage.setItem("cgm.gallery.media-layout", "JUSTIFIED");
    renderPage(queryMocks({ initialEntry: "/gallery/gallery-one?item=photo-2" }), "/gallery/gallery-one?item=photo-2");
    expect(await screen.findByRole("button", { name: "Justified rows" })).toHaveAttribute("aria-pressed", "true");
    await waitFor(() => expect(document.querySelectorAll(".media-tile")).toHaveLength(4));
    expect([...document.querySelectorAll(".media-tile")].map((tile) => (tile as HTMLElement).dataset.itemUuid)).toEqual(members.map((item) => item.itemUUID));
    expect(await screen.findByRole("dialog")).toBeInTheDocument();
  });
  it("renders ordered media groups without visible category headings", async () => {
    renderPage(queryMocks());
    expect(await screen.findByRole("heading", { name: "Gallery one" })).toBeInTheDocument();
    expect(document.querySelector(".gallery-hero")).not.toBeInTheDocument();
    await waitFor(() => expect(document.querySelectorAll(".media-tile")).toHaveLength(4));
    expect(screen.queryByRole("heading", { name: "Photos & selfies" })).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "GIF" })).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Video" })).not.toBeInTheDocument();
    expect(document.querySelectorAll(".media-tile__cover")).toHaveLength(1);
    const coverMarker = screen.getByRole("img", { name: "Current cover" });
    expect(coverMarker).toHaveAttribute("title", "Current cover");
    expect(coverMarker.textContent).toBe("");
    expect(coverMarker.querySelector("svg")).not.toBeNull();
  });

  it("shows linked tags below the gallery stats with all-scope tag browsing", async () => {
    renderPage(queryMocks());
    const heading = await screen.findByRole("heading", { name: "Gallery one" });
    const title = heading.closest(".gallery-detail__title") as HTMLElement;
    const stats = title.querySelector(".gallery-detail__stats");
    const tags = within(title).getByLabelText("Tags");
    expect(stats?.nextElementSibling).toContainElement(tags);
    expect(within(tags).getByRole("link", { name: "Outdoor" })).toHaveAttribute("href", "/tag/tag-1");
    expect(within(tags).getByRole("button", { name: "Edit tags" })).toHaveTextContent("+TAG");
    expect(tags.lastElementChild).toBe(within(tags).getByRole("button", { name: "Edit tags" }));
  });

  it("shows separate image and video dates on one stats row and omits the timeline date", async () => {
    const mocks = queryMocks();
    mocks[0] = { request: { query: GALLERY_DETAIL, variables: { slug: "gallery-one" } }, result: { data: { galleryDetail: {
      ...detail(), imageCaptureStart: "2024-05-12", imageCaptureEnd: "2024-05-15", videoCaptureStart: "2024-06-01", videoCaptureEnd: "2024-06-01",
    } } } };
    renderPage(mocks);
    const heading = await screen.findByRole("heading", { name: "Gallery one" });
    const stats = heading.closest(".gallery-detail__title")?.querySelector(".gallery-detail__stats") as HTMLElement;
    expect(within(stats).getByLabelText("Photo capture date: 2024-05-12 – 2024-05-15")).toBeInTheDocument();
    expect(within(stats).getByLabelText("Video capture date: 2024-06-01")).toBeInTheDocument();
    expect(stats).not.toHaveTextContent("2026-07");
  });

  it("orders linked works and characters before count and dates with reference-style icons", async () => {
    const mocks = queryMocks();
    mocks[0] = { request: { query: GALLERY_DETAIL, variables: { slug: "gallery-one" } }, result: { data: { galleryDetail: {
      ...detail(), imageCaptureStart: "2024-05-12", imageCaptureEnd: "2024-05-12",
      card: { ...card(), publishDate: "2025-06", publishDatePrecision: "MONTH", mediaAddedStartUTC: "2024-06-01T00:00:00Z", mediaAddedEndUTC: "2024-06-01T00:00:00Z", mediaAddedStatus: "COMPLETE" },
    } } } };
    renderPage(mocks);
    const heading = await screen.findByRole("heading", { name: "Gallery one" });
    const stats = heading.closest(".gallery-detail__title")?.querySelector(".gallery-detail__stats") as HTMLElement;
    expect(within(stats).getByRole("link", { name: "Fate" })).toHaveAttribute("href", "/work/work-1");
    expect(within(stats).getByRole("link", { name: "Saber" })).toHaveAttribute("href", "/character/character-1");
    expect(Array.from(stats.children).map((item) => item.getAttribute("title"))).toEqual([
      "Works", "Characters", "Media count", "Photo capture date", "Static image file modification time", "Publication date",
    ]);
    expect(stats.children[0].querySelector("svg path")?.getAttribute("d")).toContain("M12 5v16");
    expect(stats.children[1].querySelector("svg circle")?.getAttribute("r")).toBe("5");
    expect(stats.lastElementChild?.querySelector("svg path")?.getAttribute("d")).toContain("M8 2v3");
  });

  it("keeps every associated work and character accessible in the compact row", async () => {
    const mocks = queryMocks();
    mocks[0] = { request: { query: GALLERY_DETAIL, variables: { slug: "gallery-one" } }, result: { data: { galleryDetail: {
      ...detail(), card: { ...card(), works: [{ uuid: "work-1", name: "Fate" }, { uuid: "work-2", name: "Other work" }],
        characters: [{ uuid: "character-1", name: "Saber" }, { uuid: "character-2", name: "Other character" }] },
    } } } };
    renderPage(mocks);
    const heading = await screen.findByRole("heading", { name: "Gallery one" });
    const stats = heading.closest(".gallery-detail__title")?.querySelector(".gallery-detail__stats") as HTMLElement;
    expect(within(stats).getByRole("link", { name: "Other work" })).toHaveAttribute("href", "/work/work-2");
    expect(within(stats).getByRole("link", { name: "Other character" })).toHaveAttribute("href", "/character/character-2");
  });

  it("shows a complete static-image modification range and hides partial evidence", async () => {
    const complete = queryMocks();
    complete[0] = { request: { query: GALLERY_DETAIL, variables: { slug: "gallery-one" } }, result: { data: { galleryDetail: {
      ...detail(), card: { ...card(), mediaAddedStartUTC: "2024-01-02T03:04:05Z", mediaAddedEndUTC: "2024-02-03T04:05:06Z", mediaAddedStatus: "COMPLETE" },
    } } } };
    const rendered = renderPage(complete);
    expect(await screen.findByLabelText("Static image file modification time: 2024-01-02T03:04:05Z – 2024-02-03T04:05:06Z")).toBeInTheDocument();
    rendered.unmount();

    const partial = queryMocks();
    partial[0] = { request: { query: GALLERY_DETAIL, variables: { slug: "gallery-one" } }, result: { data: { galleryDetail: {
      ...detail(), card: { ...card(), mediaAddedStartUTC: "2024-01-02T03:04:05Z", mediaAddedEndUTC: "", mediaAddedStatus: "PARTIAL" },
    } } } };
    renderPage(partial);
    await screen.findByRole("heading", { name: "Gallery one" });
    expect(screen.queryByTitle("Static image file modification time")).not.toBeInTheDocument();
  });

  it("shows a manual publication month in the same time row", async () => {
    const mocks = queryMocks();
    mocks[0] = { request: { query: GALLERY_DETAIL, variables: { slug: "gallery-one" } }, result: { data: { galleryDetail: { ...detail(), card: { ...card(), publishDate: "2025-06", publishDatePrecision: "MONTH" } } } } };
    renderPage(mocks);
    const heading = await screen.findByRole("heading", { name: "Gallery one" });
    const stats = heading.closest(".gallery-detail__title")?.querySelector(".gallery-detail__stats") as HTMLElement;
    expect(within(stats).getByLabelText("Publication date: 2025-06")).toBeInTheDocument();
  });

  it("keeps the inline Tag editor available when the gallery has no tags", async () => {
    const mocks = queryMocks();
    mocks[0] = { request: { query: GALLERY_DETAIL, variables: { slug: "gallery-one" } }, result: { data: { galleryDetail: { ...detail(), tags: [] } } } };
    renderPage(mocks);
    const tags = await screen.findByLabelText("Tags");
    expect(within(tags).queryAllByRole("link")).toHaveLength(0);
    fireEvent.click(within(tags).getByRole("button", { name: "Edit tags" }));
    expect(await screen.findByRole("combobox", { name: "Search tags by name or alias" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save tags" })).toBeDisabled();
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

  it("edits existing Gallery tags with a searchable Stash-style chip control", async () => {
    const mocks = queryMocks();
    mocks.push(
      { request: { query: BROWSE_TAG_OPTIONS, variables: { query: "", limit: 20 } }, result: { data: { manageCoreEntityOptions: [
        { uuid: "tag-1", name: "Outdoor", aliases: [] },
        { uuid: "tag-2", name: "Portrait", aliases: ["People"] },
      ] } } },
      { request: { query: BROWSE_TAG_OPTIONS, variables: { query: "por", limit: 20 } }, result: { data: { manageCoreEntityOptions: [
        { uuid: "tag-2", name: "Portrait", aliases: ["People"] },
      ] } } },
      { request: { query: REPLACE_GALLERY_TAGS, variables: { setID: "gallery-1", expectedMetadataRevision: 7, tags: [
        { tagUUID: "tag-1", position: "1024" }, { tagUUID: "tag-2", position: "2048" },
      ] } }, result: { data: { replaceGalleryTags: { metadataRevision: 8, tags: [
        { uuid: "tag-1", name: "Outdoor" }, { uuid: "tag-2", name: "Portrait" },
      ] } } } },
      { request: { query: RELATED_GALLERIES, variables: { setID: "gallery-1" } }, result: { data: { relatedGalleries: [] } } },
    );
    renderPage(mocks);
    const moreDetails = (await screen.findByText("More details")).closest("details") as HTMLDetailsElement;
    fireEvent.click(screen.getByRole("button", { name: "Edit tags" }));
    expect(moreDetails).not.toHaveAttribute("open");
    const input = await screen.findByRole("combobox", { name: "Search tags by name or alias" });
    fireEvent.change(input, { target: { value: "por" } });
    const option = await screen.findByRole("option", { name: /Portrait/ });
    fireEvent.click(option);
    expect(document.querySelectorAll(".gallery-tag-editor__chip")).toHaveLength(2);
    fireEvent.click(screen.getByRole("button", { name: "Save tags" }));
    expect(await screen.findByText("Tags saved. The Gallery state was not changed.")).toBeInTheDocument();
    expect(within(screen.getByLabelText("Tags")).getByRole("link", { name: "Portrait" })).toHaveAttribute("href", "/tag/tag-2");
    expect(screen.getByRole("button", { name: "Edit tags" })).toHaveTextContent("+TAG");
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
