import "@testing-library/jest-dom/vitest";

import type { MockedResponse } from "@apollo/client/testing";
import { MockedProvider } from "@apollo/client/testing/react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { afterEach, describe, expect, it, vi } from "vitest";

import { DELETE_GALLERY, PREVIEW_GALLERY_DELETE } from "../api/manage";
import { messages } from "../i18n/messages";
import { ManageGalleryDeletePanel } from "./ManageGalleryDeletePanel";

const setID = "bd4e9435-2e36-4976-8b21-740e81c69450";

afterEach(cleanup);

function renderPanel(mocks: ReadonlyArray<MockedResponse>, onDeleted = vi.fn()) {
  return render(<IntlProvider locale="en-GB" messages={messages["en-GB"]}>
    <MockedProvider mocks={mocks}>
      <ManageGalleryDeletePanel setID={setID} title="Archived set" onDeleted={onDeleted} />
    </MockedProvider>
  </IntlProvider>);
}

describe("ManageGalleryDeletePanel", () => {
  it("keeps permanent deletion disabled until the Gallery is archived", async () => {
    renderPanel([{
      request: { query: PREVIEW_GALLERY_DELETE, variables: { setID } },
      result: { data: { previewGalleryDelete: {
        setID, state: "DRAFT", metadataRevision: 4, itemCount: 3, externalLinkCount: 1,
        executableJobCount: 2, ignoredSourceWillBeCreated: true, canDelete: false,
      } } },
    }]);

    expect(await screen.findByText("Archive this Gallery before permanent deletion can be enabled.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Delete archived Gallery…" })).toBeDisabled();
    expect(screen.getByText("3")).toBeInTheDocument();
  });

  it("requires both the owner password and DELETE phrase", async () => {
    const onDeleted = vi.fn();
    const preview = {
      setID, state: "ARCHIVED", metadataRevision: 7, itemCount: 3, externalLinkCount: 1,
      executableJobCount: 2, ignoredSourceWillBeCreated: true, canDelete: true,
    };
    renderPanel([
      {
        request: { query: PREVIEW_GALLERY_DELETE, variables: { setID } },
        result: { data: { previewGalleryDelete: preview } },
      },
      {
        request: { query: DELETE_GALLERY, variables: {
          setID, expectedMetadataRevision: 7, password: "owner secret", confirmation: "DELETE",
        } },
        result: { data: { deleteGallery: true } },
      },
    ], onDeleted);

    fireEvent.click(await screen.findByRole("button", { name: "Delete archived Gallery…" }));
    const commit = screen.getByRole("button", { name: "Permanently delete Gallery" });
    expect(commit).toBeDisabled();
    fireEvent.change(screen.getByLabelText("Re-enter the owner password"), { target: { value: "owner secret" } });
    expect(commit).toBeDisabled();
    fireEvent.change(screen.getByLabelText("Type DELETE to continue"), { target: { value: "DELETE" } });
    fireEvent.click(commit);
    await waitFor(() => expect(onDeleted).toHaveBeenCalledOnce());
  });
});
