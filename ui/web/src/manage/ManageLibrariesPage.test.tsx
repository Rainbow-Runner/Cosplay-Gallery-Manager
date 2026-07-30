import "@testing-library/jest-dom/vitest";

import type { MockedResponse } from "@apollo/client/testing";
import { MockedProvider } from "@apollo/client/testing/react";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it } from "vitest";

import { DELETE_RECOGNITION_RULE, MANAGE_DISCOVERY, MANAGE_LIBRARIES, UPDATE_RECOGNITION_RULE } from "../api/manage";
import { ManageLibrariesPage } from "./ManageLibrariesPage";

afterEach(cleanup);

const markerRule = {
  id: 3, name: "Marker", kind: "MARKER", enabled: true,
  autoCreateDraft: true, order: 10, pattern: "", fixedDepth: 0,
};
const library = {
  id: 2, name: "Collection", rootPath: "/media/collection", enabled: true,
  readOnly: true, captureTimezone: "Asia/Shanghai", rules: [markerRule],
};
const discoveryMock: MockedResponse = {
  request: { query: MANAGE_DISCOVERY, variables: { libraryID: 2 } },
  result: { data: { manageDiscovery: {
    id: 1, libraryID: 2, completedAt: "2026-07-27T15:00:00Z", candidates: [], unassigned: [],
  } } },
};

function renderPage(mocks: ReadonlyArray<MockedResponse>) {
  return render(<MemoryRouter>
    <MockedProvider mocks={mocks}>
      <ManageLibrariesPage />
    </MockedProvider>
  </MemoryRouter>);
}

describe("ManageLibrariesPage recognition rules", () => {
  it("loads an existing rule into the editor and saves all deterministic fields", async () => {
    renderPage([
      { request: { query: MANAGE_LIBRARIES }, result: { data: { manageLibraries: [library] } } },
      discoveryMock,
      {
        request: { query: UPDATE_RECOGNITION_RULE, variables: { input: {
          id: 3, name: "Marker updated", kind: "MARKER", enabled: true,
          autoCreateDraft: true, order: 10, pattern: "", fixedDepth: 0,
        } } },
        result: { data: { updateRecognitionRule: { ...markerRule, name: "Marker updated" } } },
      },
      { request: { query: MANAGE_LIBRARIES }, result: { data: { manageLibraries: [{ ...library, rules: [{ ...markerRule, name: "Marker updated" }] }] } } },
    ]);

    fireEvent.click(await screen.findByRole("button", { name: "Edit" }));
    const editor = screen.getByText("Edit rule #3").closest("details");
    expect(editor).not.toBeNull();
    const fields = within(editor as HTMLElement);
    fireEvent.change(fields.getByLabelText("Name (required)"), { target: { value: "Marker updated" } });
    fireEvent.click(fields.getByRole("button", { name: "Save rule" }));
    expect(await screen.findByText("Recognition rule updated. Scan the library to refresh discovery.")).toBeInTheDocument();
  });

  it("requires a second explicit action before deleting a rule", async () => {
    renderPage([
      { request: { query: MANAGE_LIBRARIES }, result: { data: { manageLibraries: [library] } } },
      discoveryMock,
      {
        request: { query: DELETE_RECOGNITION_RULE, variables: { id: 3 } },
        result: { data: { deleteRecognitionRule: true } },
      },
      { request: { query: MANAGE_LIBRARIES }, result: { data: { manageLibraries: [{ ...library, rules: [] }] } } },
    ]);

    fireEvent.click(await screen.findByRole("button", { name: "Delete" }));
    expect(screen.getByRole("button", { name: "Confirm delete" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Confirm delete" }));
    await waitFor(() => expect(screen.getByText("Recognition rule deleted. Existing Galleries are unchanged; scan the library to refresh discovery.")).toBeInTheDocument());
  });
});
