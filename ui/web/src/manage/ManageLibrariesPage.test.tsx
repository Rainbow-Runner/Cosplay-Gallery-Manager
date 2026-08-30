import "@testing-library/jest-dom/vitest";

import type { MockedResponse } from "@apollo/client/testing";
import { MockedProvider } from "@apollo/client/testing/react";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it } from "vitest";
import { IntlProvider } from "react-intl";

import { CANCEL_LIBRARY_AUTOMATION, DELETE_RECOGNITION_RULE, MANAGE_DISCOVERY, MANAGE_LIBRARIES, MANAGE_LIBRARY_AUTOMATION, SAVE_LIBRARY_AUTOMATION_POLICY, UPDATE_RECOGNITION_RULE } from "../api/manage";
import { messages } from "../i18n/messages";
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
    coverageSummary: { regularFileCount: 0, supportedMediaCount: 0, supportedArchiveCount: 0, unsupportedArchiveCount: 0, controlFileCount: 0, ignoredOtherCount: 0, actionableIssueCount: 0, registeredSourceCount: 0, indexedItemCount: 0, sourceNeedsScanCount: 0 }, coverageDiagnostics: [],
  } } },
};
const manualAutomation = {
  __typename: "ManageLibraryAutomation",
  policy: { __typename: "ManageLibraryAutomationPolicy", libraryID: 2, mode: "MANUAL", defaultContentRating: null, excludeNewRootMedia: true, autoAcceptUniqueEntities: false, autoAcceptMediaClassification: false, autoActivate: false, revision: 0 },
  preview: { __typename: "ManageLibraryAutomationPreview", candidateCount: 0, autoCreateEligible: 0, draftCount: 0, activationReady: 0, needsReview: 0 }, recentRuns: [],
};

function renderPage(mocks: ReadonlyArray<MockedResponse>) {
  return render(<MemoryRouter>
    <MockedProvider mocks={mocks}><IntlProvider locale="en-GB" messages={messages["en-GB"]}>
      <ManageLibrariesPage />
    </IntlProvider></MockedProvider>
  </MemoryRouter>);
}

describe("ManageLibrariesPage recognition rules", () => {
  it("keeps automation opt-in and saves an assisted policy explicitly", async () => {
    renderPage([
      { request: { query: MANAGE_LIBRARIES }, result: { data: { manageLibraries: [library] } } },
      discoveryMock,
      { request: { query: MANAGE_LIBRARY_AUTOMATION, variables: { libraryID: 2 } }, result: { data: { manageLibraryAutomation: manualAutomation } }, maxUsageCount: 2 },
      {
        request: { query: SAVE_LIBRARY_AUTOMATION_POLICY, variables: { libraryID: 2, expectedRevision: 0, input: {
          mode: "ASSISTED", defaultContentRating: null, excludeNewRootMedia: true,
          autoAcceptUniqueEntities: false, autoAcceptMediaClassification: false, autoActivate: false,
        } } },
        result: { data: { saveLibraryAutomationPolicy: { ...manualAutomation, policy: { ...manualAutomation.policy, mode: "ASSISTED", revision: 1 } } } },
      },
    ]);

    expect(await screen.findByRole("button", { name: "Queue automation" }, { timeout: 5000 })).toBeDisabled();
    expect(await screen.findByRole("heading", { name: "Library coverage" })).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Automation level"), { target: { value: "ASSISTED" } });
    fireEvent.click(screen.getByRole("button", { name: "Save automation policy" }));
    expect(await screen.findByText("Automation policy saved.")).toBeInTheDocument();
  });

  it("shows background progress and requests cancellation", async () => {
    const activeRun = { __typename: "ManageLibraryAutomationRun", id: 9, libraryID: 2, policyRevision: 1, mode: "ASSISTED", status: "RUNNING", cancellationRequested: false,
      candidatesSeen: 50, draftsCreated: 30, scanned: 12, activated: 0, needsReview: 12, issueCount: 2, errorCode: "", startedAt: "2026-08-29T06:00:00Z", completedAt: null };
    const activeState = { ...manualAutomation, policy: { ...manualAutomation.policy, mode: "ASSISTED", revision: 1 }, recentRuns: [activeRun] };
    renderPage([
      { request: { query: MANAGE_LIBRARIES }, result: { data: { manageLibraries: [library] } } },
      discoveryMock,
      { request: { query: MANAGE_LIBRARY_AUTOMATION, variables: { libraryID: 2 } }, result: { data: { manageLibraryAutomation: activeState } } },
      { request: { query: CANCEL_LIBRARY_AUTOMATION, variables: { runID: 9 } }, result: { data: { cancelLibraryAutomation: { __typename: "ManageLibraryAutomationRun", id: 9, status: "RUNNING", cancellationRequested: true, completedAt: null } } } },
      { request: { query: MANAGE_LIBRARY_AUTOMATION, variables: { libraryID: 2 } }, result: { data: { manageLibraryAutomation: { ...activeState, recentRuns: [{ ...activeRun, cancellationRequested: true }] } } } },
    ]);

    expect(await screen.findByText("Last run: RUNNING · scanned 12 · activated 0 · needs review 12", {}, { timeout: 5000 })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Cancel run" }));
    expect(await screen.findByText("Cancellation requested.")).toBeInTheDocument();
  });

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
    const editor = screen.getByText("Edit discovery rule #3").closest("details");
    expect(editor).not.toBeNull();
    const fields = within(editor as HTMLElement);
    fireEvent.change(fields.getByLabelText("Name (required)"), { target: { value: "Marker updated" } });
    fireEvent.click(fields.getByRole("button", { name: "Save rule" }));
    expect(await screen.findByText("Discovery rule updated. Scan the library to refresh discovery.")).toBeInTheDocument();
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
    await waitFor(() => expect(screen.getByText("Discovery rule deleted. Existing Galleries are unchanged; scan the library to refresh discovery.")).toBeInTheDocument());
  });
});
