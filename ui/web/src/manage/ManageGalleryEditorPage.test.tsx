import "@testing-library/jest-dom/vitest";

import type { MockedResponse } from "@apollo/client/testing";
import { MockedProvider } from "@apollo/client/testing/react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, describe, expect, it } from "vitest";

import { MANAGE_GALLERY, MANAGE_GALLERY_MANIFEST } from "../api/manage";
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
  aliases: [], description: "", shootDate: "", shootDatePrecision: "UNKNOWN",
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

function renderPage(mocks: ReadonlyArray<MockedResponse>, tab = "cast") {
  return render(<MemoryRouter initialEntries={[`/manage/gallery/${setID}?tab=${tab}`]}>
    <MockedProvider mocks={mocks}>
      <Routes><Route path="/manage/gallery/:setID" element={<ManageGalleryEditorPage />} /></Routes>
    </MockedProvider>
  </MemoryRouter>);
}

describe("ManageGalleryEditorPage relations", () => {
  it("loads persisted Credit/Cast from the server and marks matching folder hints as saved", async () => {
    renderPage([{
      request: { query: MANAGE_GALLERY, variables: { setID } },
      result: { data: { manageGallery: gallery } },
    }]);

    expect(await screen.findAllByText("Alice")).toHaveLength(2);
    expect(screen.getAllByText("Fate · Saber")).toHaveLength(2);
    expect(screen.getAllByText("Already saved")).toHaveLength(3);
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
