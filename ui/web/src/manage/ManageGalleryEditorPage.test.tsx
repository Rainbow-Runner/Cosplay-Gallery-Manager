import "@testing-library/jest-dom/vitest";

import type { MockedResponse } from "@apollo/client/testing";
import { MockedProvider } from "@apollo/client/testing/react";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { IntlProvider } from "react-intl";
import { afterEach, describe, expect, it, vi } from "vitest";

import { MANAGE_GALLERY, MANAGE_GALLERY_MANIFEST, REPLACE_GALLERY_RELATIONS } from "../api/manage";
import { messages } from "../i18n/messages";
import { ManageGalleryEditorPage } from "./ManageGalleryEditorPage";

afterEach(cleanup);

const setID = "018f4c8e-7a9b-7def-8123-456789abcdef";
const gallery = {
  __typename: "ManageGalleryDetail",
  row: {
    setID, slug: "gallery", state: "DRAFT", title: "Alice Fate Saber",
    contentRating: "NON_ADULT", metadataRevision: 7, scanRevision: 1, browsable: false,
    sourceType: "DIRECTORY", sourcePath: "/media/Alice Fate Saber", sourceAvailability: "AVAILABLE",
    reconcileState: "CLEAN", overLimit: false, itemCount: 1, missingCount: 0,
    pendingCount: 0, errorCount: 0, blockingIssues: 0, lastScanErrorCode: "", lastScanCompleted: "",
  },
  aliases: [], description: "", shootDate: "", shootDatePrecision: "UNKNOWN", publishDate: "", publishDatePrecision: "UNKNOWN",
  mediaAddedStartUTC: "", mediaAddedEndUTC: "", mediaAddedStatus: "NONE",
  imageCaptureStart: "", imageCaptureEnd: "", videoCaptureStart: "", videoCaptureEnd: "", captureDateCandidate: "", captureDateReviewStatus: "",
  photographerName: "", studioName: "", items: [], tags: [], externalLinks: [],
  credits: [{
    coserUUID: "coser-1", coserName: "Alice", position: "1024",
    cast: [{ characterUUID: "character-1", characterName: "Saber", workUUID: "work-1", workName: "Fate", position: "1024" }],
  }],
  folderMatches: [
    { kind: "COSER", uuid: "coser-1", name: "Alice", matchedName: "Alice", workUUID: "", workName: "" },
    { kind: "WORK", uuid: "work-1", name: "Fate", matchedName: "Fate", workUUID: "work-1", workName: "Fate" },
    { kind: "CHARACTER", uuid: "character-1", name: "Saber", matchedName: "Saber", workUUID: "work-1", workName: "Fate" },
  ],
  scanRuns: [],
};

function renderPage(mocks: ReadonlyArray<MockedResponse>, tab = "cast", locale: "en-GB" | "zh-CN" = "en-GB") {
  return render(<IntlProvider locale={locale} messages={messages[locale]}><MemoryRouter initialEntries={[`/manage/gallery/${setID}?tab=${tab}`]}>
    <MockedProvider mocks={mocks}>
      <Routes><Route path="/manage/gallery/:setID" element={<ManageGalleryEditorPage />} /></Routes>
    </MockedProvider>
  </MemoryRouter></IntlProvider>);
}

describe("ManageGalleryEditorPage relations", () => {
  it("moves a character between existing cosers only in the draft and persists on explicit save", async () => {
    const otherCast = { characterUUID: "character-2", characterName: "Rin", workUUID: "work-1", workName: "Fate", position: "1024" };
    const bob = { coserUUID: "coser-2", coserName: "Bob", position: "2048", cast: [otherCast] };
    const tags = [{ uuid: "tag-1", name: "Portrait", position: "1024" }];
    const initial = { ...gallery, credits: [...gallery.credits, bob], tags };
    const saved = { ...initial, row: { ...initial.row, metadataRevision: 8 }, credits: [{ ...gallery.credits[0], cast: [] }, { ...bob, cast: [otherCast, { ...gallery.credits[0].cast[0], position: "2048" }] }] };
    const mutation = vi.fn(() => ({ data: { replaceGalleryRelations: saved } }));
    renderPage([
      { request: { query: MANAGE_GALLERY, variables: { setID } }, result: { data: { manageGallery: initial } } },
      { request: { query: REPLACE_GALLERY_RELATIONS, variables: { setID, expectedMetadataRevision: 7, input: { credits: [
        { coserUUID: "coser-1", position: "1024", cast: [] },
        { coserUUID: "coser-2", position: "2048", cast: [{ characterUUID: "character-2", position: "1024" }, { characterUUID: "character-1", position: "2048" }] },
      ], tags: [{ tagUUID: "tag-1", position: "1024" }] } } }, result: mutation },
      { request: { query: MANAGE_GALLERY, variables: { setID } }, result: { data: { manageGallery: saved } } },
    ]);
    const owner = await screen.findByRole("combobox", { name: "Assigned Coser for Saber" });
    expect(owner).toHaveValue("0");
    fireEvent.change(owner, { target: { value: "1" } });
    const aliceArticle = screen.getByText("Alice", { selector: ".manage-entity-selector > span" }).closest("article")!;
    const bobArticle = screen.getByText("Bob", { selector: ".manage-entity-selector > span" }).closest("article")!;
    expect(within(aliceArticle).queryByText("Fate · Saber")).not.toBeInTheDocument();
    expect(within(bobArticle).getByText("Fate · Saber")).toBeInTheDocument();
    expect(within(bobArticle).getByText("Fate · Rin")).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "Assigned Coser for Saber" })).toHaveValue("1");
    expect(screen.getByText(/Character reassigned in the draft/)).toBeInTheDocument();
    expect(mutation).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Save all relations" }));
    await screen.findByText("People, characters and tags saved");
    expect(mutation).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("combobox", { name: "Assigned Coser for Saber" })).toHaveValue("1");
  });

  it("disables duplicate and unselected target cosers without losing the existing cast", async () => {
    const initial = { ...gallery, credits: [gallery.credits[0], { ...gallery.credits[0], coserUUID: "coser-2", coserName: "Bob", position: "2048" }, { coserUUID: "", coserName: "", position: "3072", cast: [] }] };
    renderPage([{ request: { query: MANAGE_GALLERY, variables: { setID } }, result: { data: { manageGallery: initial } } }], "cast", "zh-CN");
    const owner = (await screen.findAllByRole("combobox", { name: "Saber的所属人物" }))[0];
    expect(within(owner).getByRole("option", { name: "Bob (已有该角色)" })).toBeDisabled();
    expect(within(owner).getByRole("option", { name: "Coser 3" })).toBeDisabled();
    fireEvent.change(owner, { target: { value: "1" } });
    expect(owner).toHaveValue("0");
    expect(screen.getAllByText("Fate · Saber", { selector: ".manage-entity-selector > span" })).toHaveLength(2);
  });

  it("applies a Coser-only archive hint to the draft and writes only on explicit save", async () => {
    const initial = { ...gallery, row: { ...gallery.row, sourceType: "ARCHIVE", sourcePath: "/media/Alice.7z" }, credits: [], folderMatches: [gallery.folderMatches[0]] };
    const saved = { ...initial, row: { ...initial.row, metadataRevision: 8 }, credits: [{ coserUUID: "coser-1", coserName: "Alice", position: "1024", cast: [] }] };
    const mutation = vi.fn(() => ({ data: { replaceGalleryRelations: saved } }));
    renderPage([
      { request: { query: MANAGE_GALLERY, variables: { setID } }, result: { data: { manageGallery: initial } } },
      { request: { query: REPLACE_GALLERY_RELATIONS, variables: { setID, expectedMetadataRevision: 7, input: { credits: [{ coserUUID: "coser-1", position: "1024", cast: [] }], tags: [] } } }, result: mutation },
      { request: { query: MANAGE_GALLERY, variables: { setID } }, result: { data: { manageGallery: saved } } },
    ]);
    fireEvent.click(await screen.findByRole("button", { name: "Use" }));
    expect(screen.getByText("Already in relations")).toBeInTheDocument();
    expect(screen.getByText(/Match added to the relation draft/)).toBeInTheDocument();
    expect(mutation).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Save all relations" }));
    await waitFor(() => expect(mutation).toHaveBeenCalledTimes(1));
    expect(await screen.findByText("People, characters and tags saved")).toBeInTheDocument();
  });

  it("shows Character independently without a Coser but requires one before applying", async () => {
    renderPage([{ request: { query: MANAGE_GALLERY, variables: { setID } }, result: { data: { manageGallery: { ...gallery, credits: [], folderMatches: [gallery.folderMatches[2]] } } } }]);
    expect(await screen.findByText("Fate · Saber")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Use" })).toBeDisabled();
    expect(screen.getByText(/Add or select the first Coser/)).toBeInTheDocument();
  });

  it("renders source-match review controls in Chinese", async () => {
    renderPage([{ request: { query: MANAGE_GALLERY, variables: { setID } }, result: { data: { manageGallery: { ...gallery, credits: [], folderMatches: [gallery.folderMatches[0]] } } } }], "cast", "zh-CN");
    expect(await screen.findByText("来源名称匹配建议")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "应用" }));
    expect(screen.getByText("已在当前关系中")).toBeInTheDocument();
    expect(screen.getByText("已应用到关系草稿，请保存全部关系以确认")).toBeInTheDocument();
  });

  it("applies Character to the first of multiple cosers without dropping other credits or tags", async () => {
    const initial = { ...gallery, credits: [
      { coserUUID: "coser-1", coserName: "Alice", position: "1024", cast: [] },
      { coserUUID: "coser-2", coserName: "Bob", position: "2048", cast: [] },
    ], tags: [{ uuid: "tag-1", name: "Existing tag", position: "1024" }], folderMatches: gallery.folderMatches.slice(1) };
    const saved = { ...initial, row: { ...initial.row, metadataRevision: 8 }, credits: [{ ...initial.credits[0], cast: gallery.credits[0].cast }, initial.credits[1]] };
    const mutation = vi.fn(() => ({ data: { replaceGalleryRelations: saved } }));
    renderPage([
      { request: { query: MANAGE_GALLERY, variables: { setID } }, result: { data: { manageGallery: initial } } },
      { request: { query: REPLACE_GALLERY_RELATIONS, variables: { setID, expectedMetadataRevision: 7, input: { credits: [
        { coserUUID: "coser-1", position: "1024", cast: [{ characterUUID: "character-1", position: "1024" }] },
        { coserUUID: "coser-2", position: "2048", cast: [] },
      ], tags: [{ tagUUID: "tag-1", position: "1024" }] } } }, result: mutation },
      { request: { query: MANAGE_GALLERY, variables: { setID } }, result: { data: { manageGallery: saved } } },
    ]);
    expect(await screen.findByText("Context only")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Use" }));
    const credits = document.querySelectorAll(".manage-credit");
    expect(credits).toHaveLength(2);
    expect(within(credits[0] as HTMLElement).getByText("Fate · Saber")).toBeInTheDocument();
    expect(within(credits[1] as HTMLElement).getByText("Bob")).toBeInTheDocument();
    expect(screen.getByText("Existing tag")).toBeInTheDocument();
    expect(mutation).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Save all relations" }));
    await waitFor(() => expect(mutation).toHaveBeenCalledTimes(1));
  });
  it("requires an explicit decision when extracted and manual shoot dates differ", async () => {
    const conflict = { ...gallery, shootDate: "2024-05-20", shootDatePrecision: "DAY", imageCaptureStart: "2024-05-12", imageCaptureEnd: "2024-05-15", captureDateCandidate: "2024-05-12", captureDateReviewStatus: "PENDING" };
    renderPage([{ request: { query: MANAGE_GALLERY, variables: { setID } }, result: { data: { manageGallery: conflict } } }], "basic");
    expect(await screen.findByText(/Manual shoot date 2024-05-20 differs/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Keep manual date" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Use extracted date" })).toBeEnabled();
  });
  it("loads persisted Credit/Cast from the server and marks matching folder hints as saved", async () => {
    renderPage([{
      request: { query: MANAGE_GALLERY, variables: { setID } },
      result: { data: { manageGallery: gallery } },
    }]);

    expect(await screen.findAllByText("Alice")).toHaveLength(2);
    expect(screen.getAllByText("Fate · Saber")).toHaveLength(2);
    expect(screen.getAllByText("Already in relations")).toHaveLength(2);
    expect(screen.getByText("Context only")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save all relations" })).toBeEnabled();

    fireEvent.click(screen.getByRole("button", { name: "Add Character" }));
    expect(screen.getByRole("button", { name: "Save all relations" })).toBeDisabled();
    expect(screen.getByText(/Finish or remove every empty/)).toBeInTheDocument();
  });

  it("defaults manual scans to auto-excluding only newly discovered root media", async () => {
    renderPage([{
      request: { query: MANAGE_GALLERY, variables: { setID } },
      result: { data: { manageGallery: gallery } },
    }], "source");

    const option = await screen.findByRole("checkbox", { name: /Auto-exclude newly discovered media in Gallery root/ });
    expect(option).toBeChecked();
    expect(screen.getByText(/Existing Restore\/Exclude choices are never overwritten/)).toBeInTheDocument();
    const deepScan = screen.getByRole("checkbox", { name: /Read all media content/ });
    expect(deepScan).not.toBeChecked();
    expect(screen.getByRole("button", { name: "Scan source now" })).toBeEnabled();
    fireEvent.click(deepScan);
    expect(screen.getByRole("button", { name: "Run deep scan" })).toBeEnabled();
  });

  it("makes missing members explicit and offers a database-only forget action", async () => {
		const missing = { ...gallery, row: { ...gallery.row, itemCount: 2, missingCount: 1 }, items: [
			{ uuid: "missing-item", relativePath: "old/01.jpg", mediaKind: "STATIC_IMAGE", contentFormat: "IMAGE", imageCategory: "PHOTO", position: "1024", caption: "", excluded: false, availability: "MISSING", processingState: "READY", byteSize: 100, videoProbeState: "", videoErrorCode: "", videoContainer: "", videoDurationSeconds: 0, videoWidth: 0, videoHeight: 0, videoCodec: "", audioCodec: "" },
			{ uuid: "new-item", relativePath: "new/01.jpg", mediaKind: "STATIC_IMAGE", contentFormat: "IMAGE", imageCategory: "PHOTO", position: "2048", caption: "", excluded: false, availability: "AVAILABLE", processingState: "PENDING", byteSize: 200, videoProbeState: "", videoErrorCode: "", videoContainer: "", videoDurationSeconds: 0, videoWidth: 0, videoHeight: 0, videoCodec: "", audioCodec: "" },
		] };
		renderPage([{ request: { query: MANAGE_GALLERY, variables: { setID } }, result: { data: { manageGallery: missing } } }], "media");

		expect(await screen.findByText("2 members · 1 missing · folder paths are relative to the Gallery root")).toBeInTheDocument();
		expect(screen.queryByRole("button", { name: "Forget record" })).not.toBeInTheDocument();
		fireEvent.click(screen.getByRole("button", { name: "Expand folder old" }));
		expect(screen.getByRole("button", { name: "Forget record" })).toBeInTheDocument();
		fireEvent.click(screen.getByRole("button", { name: "Replace file…" }));
		expect(screen.getByRole("heading", { name: /Confirm media replacement/ })).toBeInTheDocument();
		expect(screen.getByRole("option", { name: /new\/01.jpg/ })).toBeInTheDocument();
		fireEvent.click(screen.getByRole("checkbox", { name: /Missing only/ }));
		expect(screen.getByText("old")).toBeInTheDocument();
		expect(screen.queryByText("new")).not.toBeInTheDocument();
	});

	it("collapses nested directories while keeping the caption beside the filename", async () => {
		const photo = { uuid: "nested-item", relativePath: "disc/chapter/01.jpg", mediaKind: "STATIC_IMAGE", contentFormat: "JPEG", imageCategory: "PHOTO", position: "1024", caption: "A caption", excluded: false, availability: "AVAILABLE", processingState: "READY", byteSize: 100, videoProbeState: "", videoErrorCode: "", videoContainer: "", videoDurationSeconds: 0, videoWidth: 0, videoHeight: 0, videoCodec: "", audioCodec: "" };
		renderPage([{ request: { query: MANAGE_GALLERY, variables: { setID } }, result: { data: { manageGallery: { ...gallery, items: [photo] } } } }], "media");
		expect(await screen.findByRole("button", { name: "Expand folder disc" })).toHaveAttribute("aria-expanded", "false");
		expect(screen.queryByRole("textbox", { name: "Caption for disc/chapter/01.jpg" })).not.toBeInTheDocument();
		fireEvent.click(screen.getByRole("button", { name: "Expand folder disc" }));
		fireEvent.click(screen.getByRole("button", { name: "Expand folder disc/chapter" }));
		const caption = screen.getByRole("textbox", { name: "Caption for disc/chapter/01.jpg" });
		expect(caption.closest(".manage-media-file-line")?.querySelector("strong")?.textContent).toBe("01.jpg");
		fireEvent.click(screen.getByRole("button", { name: "Collapse folder disc" }));
		expect(caption).not.toBeInTheDocument();
	});

	it("shows the portable member diff before an explicit Manifest Push", async () => {
		renderPage([
			{ request: { query: MANAGE_GALLERY, variables: { setID } }, result: { data: { manageGallery: gallery } } },
			{ request: { query: MANAGE_GALLERY_MANIFEST, variables: { setID } }, result: { data: { manageGalleryManifest: {
				__typename: "ManageGalleryManifestState", status: "DB_DIRTY", path: "/media/.cosplay.json",
				manifestRevision: 2, metadataRevision: 7, pushAdded: 1, pushRemoved: 2, pushRetained: 3, pushUpdated: 1, conflicts: [],
			} } } },
		], "manifest");

		expect(await screen.findByText("+1 added · −2 removed · 3 retained · 1 updated")).toBeInTheDocument();
	});
});
