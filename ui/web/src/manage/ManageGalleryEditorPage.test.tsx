import "@testing-library/jest-dom/vitest";

import type { MockedResponse } from "@apollo/client/testing";
import { MockedProvider } from "@apollo/client/testing/react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, describe, expect, it } from "vitest";

import { MANAGE_GALLERY } from "../api/manage";
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
    pendingCount: 0, errorCount: 0, blockingIssues: 0,
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
};

function renderPage(mocks: ReadonlyArray<MockedResponse>) {
  return render(<MemoryRouter initialEntries={[`/manage/gallery/${setID}?tab=cast`]}>
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
});
