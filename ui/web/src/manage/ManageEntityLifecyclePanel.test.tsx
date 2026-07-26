import "@testing-library/jest-dom/vitest";

import { MockedProvider } from "@apollo/client/testing/react";
import type { MockedResponse } from "@apollo/client/testing";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { afterEach, describe, expect, it, vi } from "vitest";

import { DELETE_CORE_ENTITY, PREVIEW_CORE_ENTITY_DELETE, PREVIEW_CORE_ENTITY_MERGE } from "../api/manage";
import { messages } from "../i18n/messages";
import { ManageEntityLifecyclePanel } from "./ManageEntityLifecyclePanel";
import type { ManageCoreEntity } from "./types";

const source: ManageCoreEntity = {
  kind: "WORK",
  uuid: "35ab1f65-fde2-4f2a-92f2-bdac02a774d5",
  name: "Source Work",
  sortName: "",
  aliases: [],
  slug: "source-work",
  metadataRevision: 3,
  profileSummary: "",
  biography: "",
  countryOrRegion: "",
  useInRecommendation: false,
  socialAccounts: [],
  parents: [],
};

afterEach(cleanup);

function renderPanel(mocks: ReadonlyArray<MockedResponse>, onDeleted = vi.fn()) {
  return render(<IntlProvider locale="en-GB" messages={messages["en-GB"]}><MockedProvider mocks={mocks}><ManageEntityLifecyclePanel source={source} onMerged={vi.fn()} onDeleted={onDeleted} /></MockedProvider></IntlProvider>);
}

describe("ManageEntityLifecyclePanel", () => {
  it("shows complete merge and delete blockers before enabling destructive actions", async () => {
    const targetUUID = "9d85ad3e-8f54-47a9-83ef-ca5b2da7c329";
    const mocks = [
      {
        request: { query: PREVIEW_CORE_ENTITY_DELETE, variables: { kind: "WORK", uuid: source.uuid } },
        result: { data: { previewCoreEntityDelete: { kind: "WORK", uuid: source.uuid, metadataRevision: 3, referenceCount: 2, blockers: [{ code: "CHARACTER", referenceCount: 2 }], canDelete: false } } },
      },
      {
        request: { query: PREVIEW_CORE_ENTITY_MERGE, variables: { kind: "WORK", sourceUUID: source.uuid, targetUUID } },
        result: { data: { previewCoreEntityMerge: { __typename: "ManageCoreEntityMergePreview", kind: "WORK", sourceUUID: source.uuid, targetUUID, sourceRevision: 3, targetRevision: 5, affectedGalleryIDs: [12, 19], conflicts: [{ __typename: "ManageCoreEntityMergeConflict", code: "DUPLICATE_CHARACTER_NAME", details: "1 conflicting relationship(s)" }], canMerge: false } } },
      },
    ];
    renderPanel(mocks);

    expect(await screen.findByText(/Deletion is blocked by 2 reference/)).toBeInTheDocument();
    expect(screen.getByText("CHARACTER")).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Target WORK UUID"), { target: { value: targetUUID } });
    fireEvent.click(screen.getByRole("button", { name: "Preview merge" }));
    expect(await screen.findByText("DUPLICATE_CHARACTER_NAME")).toBeInTheDocument();
    expect(screen.getByText("12, 19")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Merge permanently…" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Delete permanently…" })).toBeDisabled();
  });

  it("requires the explicit phrase before deleting an unreferenced entity", async () => {
    const onDeleted = vi.fn();
    const mocks = [
      {
        request: { query: PREVIEW_CORE_ENTITY_DELETE, variables: { kind: "WORK", uuid: source.uuid } },
        result: { data: { previewCoreEntityDelete: { kind: "WORK", uuid: source.uuid, metadataRevision: 3, referenceCount: 0, blockers: [], canDelete: true } } },
      },
      {
        request: { query: DELETE_CORE_ENTITY, variables: { kind: "WORK", uuid: source.uuid, expectedMetadataRevision: 3 } },
        result: { data: { deleteCoreEntity: true } },
      },
    ];
    renderPanel(mocks, onDeleted);

    const open = await screen.findByRole("button", { name: "Delete permanently…" });
    fireEvent.click(open);
    const commit = screen.getByRole("button", { name: "Delete and tombstone UUID" });
    expect(commit).toBeDisabled();
    fireEvent.change(screen.getByLabelText(/Type DELETE to continue/), { target: { value: "DELETE" } });
    fireEvent.click(commit);
    await waitFor(() => expect(onDeleted).toHaveBeenCalledOnce());
  });
});
