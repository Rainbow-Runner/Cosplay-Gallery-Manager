import "@testing-library/jest-dom/vitest";

import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ManageCoserAssetReviewPanel } from "./ManageCoserAssetReviewPanel";
import type { ManageCoserAssetReview } from "./types";

const groupID = "a".repeat(64);
const review: ManageCoserAssetReview = {
  groups: [{
    id: groupID,
    coser_uuid: "2d9f6174-eaa9-45e6-8fc9-928e62b655aa",
    kind: "AVATAR",
    reason: "REPLACED",
    file_count: 2,
    byte_size: 1536,
    modified_at: "2026-07-26T12:00:00Z",
  }],
  total_file_count: 2,
  total_byte_size: 1536,
  ignored_entry_count: 1,
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("ManageCoserAssetReviewPanel", () => {
  it("requires selection, owner password and CLEAN before submitting opaque group IDs", async () => {
    const fetchMock = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(new Response(JSON.stringify(review), { status: 200, headers: { "Content-Type": "application/json" } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        deleted_group_count: 1,
        deleted_file_count: 2,
        deleted_byte_size: 1536,
        review: { groups: [], total_file_count: 0, total_byte_size: 0, ignored_entry_count: 1 },
      }), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);
    render(<ManageCoserAssetReviewPanel />);

    expect(await screen.findByText("Replaced or unpublished")).toBeInTheDocument();
    const open = screen.getByRole("button", { name: "Clean selected…" });
    expect(open).toBeDisabled();
    fireEvent.click(screen.getByLabelText(`Select asset group ${groupID}`));
    expect(open).toBeEnabled();
    fireEvent.click(open);

    const submit = screen.getByRole("button", { name: "Permanently remove selected files" });
    expect(submit).toBeDisabled();
    fireEvent.change(screen.getByLabelText("Owner password"), { target: { value: "owner secret" } });
    fireEvent.change(screen.getByLabelText(/Type CLEAN/), { target: { value: "CLEAN" } });
    expect(submit).toBeEnabled();
    fireEvent.click(submit);

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    const [url, options] = fetchMock.mock.calls[1]!;
    expect(url).toBe("/manage/coser-assets/review");
    expect(options?.method).toBe("POST");
    expect(JSON.parse(String(options?.body))).toEqual({
      asset_ids: [groupID],
      password: "owner secret",
      confirmation: "CLEAN",
    });
    expect(await screen.findByText("Removed 2 generated files (1.50 KiB).")).toBeInTheDocument();
    expect(screen.getByText("No unreferenced generated Coser assets require review.")).toBeInTheDocument();
  });
});
