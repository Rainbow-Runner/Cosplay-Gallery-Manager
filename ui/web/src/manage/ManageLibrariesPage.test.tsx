import "@testing-library/jest-dom/vitest";

import type { MockedResponse } from "@apollo/client/testing";
import { MockedProvider } from "@apollo/client/testing/react";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { IntlProvider } from "react-intl";

import { APPLY_MEDIA_LIBRARY_CHANGE, CANCEL_LIBRARY_AUTOMATION, DELETE_RECOGNITION_RULE, MANAGE_DISCOVERY, MANAGE_IGNORED_SOURCES, MANAGE_LIBRARIES, MANAGE_LIBRARY_AUTOMATION, PREVIEW_IGNORED_SOURCE_REMOVAL, PREVIEW_MEDIA_LIBRARY_CHANGE, REVOKE_IGNORED_SOURCE, RUN_LIBRARY_AUTOMATION, SAVE_LIBRARY_AUTOMATION_POLICY, SET_MEDIA_LIBRARY_METADATA_WRITEBACK, TRANSFER_MEDIA_LIBRARY_SOURCE, UPDATE_RECOGNITION_RULE } from "../api/manage";
import { messages } from "../i18n/messages";
import { ManageLibrariesPage } from "./ManageLibrariesPage";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

const markerRule = {
  id: 3, name: "Marker", kind: "MARKER", enabled: true,
  autoCreateDraft: true, order: 10, pattern: "", fixedDepth: 0,
};
const library = {
  id: 2, name: "Collection", rootPath: "/media/collection", enabled: true,
  metadataWritebackEnabled: false, boundGalleryCount: 8, updatedAt: "2026-09-18T14:00:00Z", captureTimezone: "Asia/Shanghai", rules: [markerRule],
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
  policy: { __typename: "ManageLibraryAutomationPolicy", libraryID: 2, mode: "MANUAL", defaultContentRating: null, excludeNewRootMedia: true, autoImportArchives: false, autoAcceptUniqueEntities: false, autoAcceptMediaClassification: false, autoActivate: false, revision: 0 },
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
  it("enables metadata sidecar writeback for an existing library with confirmation", async () => {
    vi.spyOn(window, "confirm").mockReturnValue(true);
    renderPage([
      { request: { query: MANAGE_LIBRARIES }, result: { data: { manageLibraries: [library] } } },
      discoveryMock,
      { request: { query: SET_MEDIA_LIBRARY_METADATA_WRITEBACK, variables: { libraryID: 2, enabled: true, expectedUpdatedAt: library.updatedAt } }, result: { data: { setMediaLibraryMetadataWriteback: { id: 2, metadataWritebackEnabled: true, updatedAt: "2026-09-18T15:00:00Z" } } } },
      { request: { query: MANAGE_LIBRARIES }, result: { data: { manageLibraries: [{ ...library, metadataWritebackEnabled: true, updatedAt: "2026-09-18T15:00:00Z" }] } } },
    ]);
    fireEvent.click(await screen.findByRole("button", { name: "Enable sidecar writeback" }));
    expect(window.confirm).toHaveBeenCalledWith(expect.stringContaining("8 bound galleries"));
    expect(await screen.findByRole("button", { name: "Disable sidecar writeback" })).toBeInTheDocument();
  });

  it("reviews a global ignore before password-confirmed removal", async () => {
    const record = { id: 7, libraryID: null, setID: null, path: "/media/collection/old", reason: "SOURCE_REBOUND", createdAt: "2026-09-14T00:00:00Z" };
    renderPage([
      { request: { query: MANAGE_LIBRARIES }, result: { data: { manageLibraries: [library] } } },
      discoveryMock,
      { request: { query: MANAGE_IGNORED_SOURCES, variables: { libraryID: null, page: 1, query: "" } }, result: { data: { manageIgnoredSources: { page: 1, pageSize: 50, total: 1, items: [record] } } } },
      { request: { query: PREVIEW_IGNORED_SOURCE_REMOVAL, variables: { id: 7 } }, result: { data: { previewIgnoredSourceRemoval: { record, affectedLibraryIDs: [2], activeRunCount: 0, boundSourceCount: 0, revisionToken: "token" } } } },
      { request: { query: REVOKE_IGNORED_SOURCE, variables: { id: 7, revisionToken: "token", password: "secret", confirmation: "REVEAL" } }, result: { data: { revokeIgnoredSource: true } } },
      { request: { query: MANAGE_IGNORED_SOURCES, variables: { libraryID: null, page: 1, query: "" } }, result: { data: { manageIgnoredSources: { page: 1, pageSize: 50, total: 0, items: [] } } } },
    ]);
    fireEvent.click(await screen.findByRole("button", { name: "加载忽略记录 / Load ignores" }));
    expect(await screen.findByText("/media/collection/old")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "预览撤销 / Preview removal" }));
    const button = await screen.findByRole("button", { name: "撤销忽略 / Revoke ignore" });
    expect(button).toBeDisabled();
    fireEvent.change(screen.getByLabelText("所有者密码 / Owner password"), { target: { value: "secret" } });
    fireEvent.change(screen.getByLabelText("输入 REVEAL 确认 / Type REVEAL"), { target: { value: "REVEAL" } });
    fireEvent.click(button);
    expect(await screen.findByText(/忽略记录已撤销/)).toBeInTheDocument();
  });
  it("previews bound Sources and requires explicit transfer confirmation", async () => {
    vi.spyOn(window, "confirm").mockReturnValue(true);
    const parent = { ...library, id: 1, name: "Parent", rootPath: "/media", rules: [] };
    const basePreview = { libraryID: 2, currentRoot: library.rootPath, proposedRoot: "", revisionToken: "token", ignoredSourceCount: 1,
      ignoredSources: [{ id: 8, path: "/media/collection/ignored", reason: "reviewed" }], unassignedSourcePaths: [], recognitionRules: [], classificationRules: [], exclusionRules: [],
      automationMode: "MANUAL", automationPolicyRevision: 0, automationRunCount: 0, activeRunCount: 0, portableMappingCount: 0, scanningSourceCount: 0, proposedBoundaryConflicts: [], childRoots: [], impacts: [
      { sourceID: 9, galleryID: 11, galleryTitle: "Set", sourcePath: "/media/collection/set", currentLibraryID: 2, suggestedOwnerID: 1 },
    ] };
    renderPage([
      { request: { query: MANAGE_LIBRARIES }, result: { data: { manageLibraries: [library, parent] } } },
      discoveryMock,
      { request: { query: PREVIEW_MEDIA_LIBRARY_CHANGE, variables: { libraryID: 2, newRoot: "" } }, result: { data: { previewMediaLibraryChange: basePreview } } },
      { request: { query: TRANSFER_MEDIA_LIBRARY_SOURCE, variables: { sourceID: 9, expectedLibraryID: 2, targetLibraryID: 1 } }, result: { data: { transferMediaLibrarySource: true } } },
      { request: { query: PREVIEW_MEDIA_LIBRARY_CHANGE, variables: { libraryID: 2, newRoot: "" } }, result: { data: { previewMediaLibraryChange: { ...basePreview, impacts: [] } } } },
    ]);
    fireEvent.click(await screen.findByRole("button", { name: "预览影响 / Preview impact" }));
    expect(await screen.findByText("Set")).toBeInTheDocument();
    const transferButton = screen.getByRole("button", { name: "确认转移 / Transfer" });
    expect(transferButton).toBeDisabled();
    fireEvent.change(screen.getByLabelText("Transfer Source 9 to"), { target: { value: "1" } });
    fireEvent.click(transferButton);
    expect(window.confirm).toHaveBeenCalledOnce();
    expect(await screen.findByText("Source binding updated / Source 归属已更新")).toBeInTheDocument();
  });

  it("requires a fresh impact preview, owner password and exact move phrase", async () => {
    const proposedRoot = "/media/new-collection";
    renderPage([
      { request: { query: MANAGE_LIBRARIES }, result: { data: { manageLibraries: [library] } } },
      discoveryMock,
      { request: { query: PREVIEW_MEDIA_LIBRARY_CHANGE, variables: { libraryID: 2, newRoot: proposedRoot } }, result: { data: { previewMediaLibraryChange: {
        libraryID: 2, currentRoot: library.rootPath, proposedRoot, revisionToken: "review-token", ignoredSourceCount: 1,
        ignoredSources: [{ id: 5, path: "/media/collection/ignored", reason: "reviewed" }], unassignedSourcePaths: [],
        recognitionRules: [{ id: 3, name: "Marker" }], classificationRules: [], exclusionRules: [],
        automationMode: "MANUAL", automationPolicyRevision: 0, automationRunCount: 0, activeRunCount: 0, portableMappingCount: 0, scanningSourceCount: 0, proposedBoundaryConflicts: [],
        childRoots: [], impacts: [],
      } } } },
      { request: { query: APPLY_MEDIA_LIBRARY_CHANGE, variables: { libraryID: 2, newRoot: proposedRoot, revisionToken: "review-token", password: "secret", confirmation: "MOVE ROOT" } }, result: { data: { applyMediaLibraryChange: true } } },
      { request: { query: MANAGE_LIBRARIES }, result: { data: { manageLibraries: [{ ...library, rootPath: proposedRoot }] } } },
    ]);
    fireEvent.change(await screen.findByLabelText("拟议的新根路径（留空预览删除） / Proposed root (blank for deletion)"), { target: { value: proposedRoot } });
    fireEvent.click(screen.getByRole("button", { name: "预览影响 / Preview impact" }));
    expect(await screen.findByText((_, element) => element?.tagName === "P" && element.textContent?.includes("#3 Marker") === true)).toBeInTheDocument();
    const button = screen.getByRole("button", { name: "更改媒体库根 / Move root" });
    expect(button).toBeDisabled();
    fireEvent.change(screen.getByLabelText("所有者密码 / Owner password"), { target: { value: "secret" } });
    fireEvent.change(screen.getByLabelText("输入确认词 / Type MOVE ROOT"), { target: { value: "MOVE ROOT" } });
    expect(button).toBeEnabled();
    fireEvent.click(button);
    expect(await screen.findByText("Media library change applied / 媒体库变更已执行")).toBeInTheDocument();
  });
  it("keeps automation opt-in and saves an assisted policy explicitly", async () => {
    renderPage([
      { request: { query: MANAGE_LIBRARIES }, result: { data: { manageLibraries: [library] } } },
      discoveryMock,
      { request: { query: MANAGE_LIBRARY_AUTOMATION, variables: { libraryID: 2 } }, result: { data: { manageLibraryAutomation: manualAutomation } }, maxUsageCount: 2 },
      {
        request: { query: SAVE_LIBRARY_AUTOMATION_POLICY, variables: { libraryID: 2, expectedRevision: 0, input: {
          mode: "ASSISTED", defaultContentRating: null, excludeNewRootMedia: true,
          autoImportArchives: false,
          autoAcceptUniqueEntities: false, autoAcceptMediaClassification: false, autoActivate: false,
        } } },
        result: { data: { saveLibraryAutomationPolicy: { ...manualAutomation, policy: { ...manualAutomation.policy, mode: "ASSISTED", revision: 1 } } } },
      },
    ]);

    expect(await screen.findByRole("button", { name: "Scan and process by saved rules" }, { timeout: 5000 })).toBeDisabled();
    expect(await screen.findByRole("heading", { name: "Library coverage" })).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Automation level"), { target: { value: "ASSISTED" } });
    fireEvent.click(screen.getByRole("button", { name: "Save automation policy" }));
    expect(await screen.findByText("Automation policy saved.")).toBeInTheDocument();
  });

  it("distinguishes review-only discovery and requires saved policy before explicit automation", async () => {
    const assistedPolicy = { ...manualAutomation.policy, mode: "ASSISTED", revision: 3 };
    const assistedAutomation = { ...manualAutomation, policy: assistedPolicy };
    const queuedRun = { __typename: "ManageLibraryAutomationRun", id: 12, libraryID: 2, policyRevision: 3, mode: "ASSISTED", status: "QUEUED", cancellationRequested: false,
      phase: "QUEUED", processedTargets: 0, totalTargets: 0, currentGalleryTitle: "",
      candidatesSeen: 0, draftsCreated: 0, scanned: 0, activated: 0, needsReview: 0, issueCount: 0, errorCode: "", startedAt: null, completedAt: null };
    renderPage([
      { request: { query: MANAGE_LIBRARIES }, result: { data: { manageLibraries: [library] } } },
      discoveryMock,
      { request: { query: MANAGE_LIBRARY_AUTOMATION, variables: { libraryID: 2 } }, result: { data: { manageLibraryAutomation: assistedAutomation } }, maxUsageCount: 2 },
      { request: { query: RUN_LIBRARY_AUTOMATION, variables: { libraryID: 2 } }, result: { data: { runLibraryAutomation: queuedRun } } },
    ]);

    await waitFor(() => expect(screen.getByRole("button", { name: "Scan and review only" })).toBeEnabled());
    const runButton = await screen.findByRole("button", { name: "Scan and process by saved rules" }, { timeout: 5000 });
    expect(runButton).toBeEnabled();
    fireEvent.click(screen.getByLabelText("Accept deterministic media classification suggestions"));
    expect(runButton).toBeDisabled();
    expect(screen.getByText(/Save the changed policy before running automation/)).toBeInTheDocument();
    fireEvent.click(screen.getByLabelText("Accept deterministic media classification suggestions"));
    expect(runButton).toBeEnabled();
    fireEvent.click(runButton);
    expect(await screen.findByText("Run #12 queued. It will scan the library first, then process the result with the saved policy.")).toBeInTheDocument();
  });

  it("shows background progress and requests cancellation", async () => {
    const activeRun = { __typename: "ManageLibraryAutomationRun", id: 9, libraryID: 2, policyRevision: 1, mode: "ASSISTED", status: "RUNNING", cancellationRequested: false,
      phase: "WAITING_FOR_MEDIA", processedTargets: 12, totalTargets: 30, currentGalleryTitle: "Miku Set",
      candidatesSeen: 50, draftsCreated: 30, scanned: 12, activated: 0, needsReview: 12, issueCount: 2, errorCode: "", startedAt: "2026-08-29T06:00:00Z", completedAt: null };
    const activeState = { ...manualAutomation, policy: { ...manualAutomation.policy, mode: "ASSISTED", revision: 1 }, recentRuns: [activeRun] };
    renderPage([
      { request: { query: MANAGE_LIBRARIES }, result: { data: { manageLibraries: [library] } } },
      discoveryMock,
      { request: { query: MANAGE_LIBRARY_AUTOMATION, variables: { libraryID: 2 } }, result: { data: { manageLibraryAutomation: activeState } } },
      { request: { query: CANCEL_LIBRARY_AUTOMATION, variables: { runID: 9 } }, result: { data: { cancelLibraryAutomation: { __typename: "ManageLibraryAutomationRun", id: 9, status: "RUNNING", cancellationRequested: true, completedAt: null } } } },
      { request: { query: MANAGE_LIBRARY_AUTOMATION, variables: { libraryID: 2 } }, result: { data: { manageLibraryAutomation: { ...activeState, recentRuns: [{ ...activeRun, cancellationRequested: true }] } } } },
    ]);

    expect(await screen.findByText("Last run: RUNNING · scanned 12 · activated 0 · Gallery review 12", {}, { timeout: 5000 })).toBeInTheDocument();
    expect(screen.getByText("Waiting for base previews")).toBeInTheDocument();
    expect(screen.getByText("12 of 30 Galleries")).toBeInTheDocument();
    expect(screen.getByRole("progressbar", { name: "Automation progress" })).toHaveAttribute("aria-valuenow", "12");
    const classification = screen.getByLabelText("Accept deterministic media classification suggestions");
    fireEvent.click(classification);
    expect(classification).toBeChecked();
    fireEvent.click(screen.getByRole("button", { name: "Cancel run" }));
    expect(await screen.findByText("Cancellation requested.")).toBeInTheDocument();
    expect(classification).toBeChecked();
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

  it("prepares an exact deterministic rule from a genuinely unassigned directory", async () => {
    const unassignedDiscovery: MockedResponse = {
      request: { query: MANAGE_DISCOVERY, variables: { libraryID: 2 } },
      result: { data: { manageDiscovery: {
        id: 2, libraryID: 2, completedAt: "2026-08-30T15:00:00Z", candidates: [],
        unassigned: [{ parentPath: "/media/collection/Loose Set", mediaCount: 12 }],
        coverageSummary: { regularFileCount: 12, supportedMediaCount: 12, supportedArchiveCount: 0, unsupportedArchiveCount: 0, controlFileCount: 0, ignoredOtherCount: 0, actionableIssueCount: 1, registeredSourceCount: 0, indexedItemCount: 0, sourceNeedsScanCount: 0 },
        coverageDiagnostics: [{ path: "/media/collection/Loose Set", entryKind: "DIRECTORY", reasonCode: "UNASSIGNED_MEDIA_DIRECTORY", fileCount: 12, byteSize: 0 }],
      } } },
    };
    renderPage([
      { request: { query: MANAGE_LIBRARIES }, result: { data: { manageLibraries: [library] } } },
      unassignedDiscovery,
      { request: { query: MANAGE_LIBRARY_AUTOMATION, variables: { libraryID: 2 } }, result: { data: { manageLibraryAutomation: manualAutomation } } },
    ]);

    fireEvent.click(await screen.findByRole("button", { name: "Prepare exact directory rule" }));
    const pattern = await screen.findByLabelText("Full relative path RE2 (required)");
    expect(pattern).toHaveValue("(?P<title>Loose Set)");
    expect(screen.getByLabelText("Enable rule")).toBeChecked();
    expect(screen.getByLabelText("Auto-create DRAFT")).not.toBeChecked();
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
