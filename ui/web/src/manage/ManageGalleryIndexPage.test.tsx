import "@testing-library/jest-dom/vitest";

import type { MockedResponse } from "@apollo/client/testing";
import { MockedProvider } from "@apollo/client/testing/react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { afterEach, describe, expect, it } from "vitest";

import { MANAGE_GALLERIES, PREVIEW_GALLERY_MANIFEST_BATCH_PUSH, PUSH_GALLERY_MANIFESTS } from "../api/manage";
import { ManageGalleryIndexPage } from "./ManageGalleryIndexPage";

afterEach(cleanup);

const summary = { all: 60, draft: 55, overLimit: 2, unavailable: 3, blocking: 4, missingGallery: 1, processingError: 2, manifestAttention: 4 };

function galleryPage(page: number, issue: string): MockedResponse {
  return {
    request: { query: MANAGE_GALLERIES, variables: { page, issue } },
    result: { data: { manageGalleries: { page, pageSize: 24, totalItems: issue === "ALL" ? 60 : issue === "MISSING" ? 1 : issue === "PROCESSING_ERROR" ? 2 : 55,
      totalPages: page === 3 ? 3 : 1, summary, items: [] } } },
  };
}

function CurrentLocation() {
  const location = useLocation();
  return <output data-testid="location">{location.search}</output>;
}

describe("ManageGalleryIndexPage filter cards", () => {
  it("uses eight keyboard-accessible cards as the only filter and resets pagination", async () => {
    render(<MemoryRouter initialEntries={["/manage/gallery?page=3&issue=DRAFT"]}>
      <MockedProvider mocks={[galleryPage(3, "DRAFT"), galleryPage(1, "MISSING"), galleryPage(1, "PROCESSING_ERROR"), galleryPage(1, "ALL")]}>
        <><ManageGalleryIndexPage /><CurrentLocation /></>
      </MockedProvider>
    </MemoryRouter>);

    expect(screen.getByRole("heading", { name: "Gallery" })).toBeInTheDocument();
    const filters = await screen.findByRole("navigation", { name: "作品集筛选 / Gallery filters" });
    expect(filters.querySelectorAll("button")).toHaveLength(8);
    expect(screen.queryByRole("combobox")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /草稿 \/ Draft/ })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: /缺失媒体 \/ Missing/ })).toHaveTextContent("1");

    fireEvent.click(screen.getByRole("button", { name: /缺失媒体 \/ Missing/ }));
    expect(await screen.findByRole("button", { name: /缺失媒体 \/ Missing/ })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByTestId("location")).toHaveTextContent("?issue=MISSING");

    fireEvent.click(screen.getByRole("button", { name: /处理失败 \/ Processing error/ }));
    expect(await screen.findByRole("button", { name: /处理失败 \/ Processing error/ })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByTestId("location")).toHaveTextContent("?issue=PROCESSING_ERROR");

    fireEvent.click(screen.getByRole("button", { name: /全部 \/ All/ }));
    expect(await screen.findByRole("button", { name: /全部 \/ All/ })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByTestId("location").textContent).toBe("");
  });

  it("previews a selected Gallery and offers explicit skip or overwrite decisions", async () => {
    const row = { setID: "gallery-one", slug: "one", state: "ACTIVE", title: "One", contentRating: "NON_ADULT", metadataRevision: 4, scanRevision: 2, browsable: true, sourceType: "DIRECTORY", sourcePath: "/media/one", sourceAvailability: "AVAILABLE", reconcileState: "IN_SYNC", overLimit: false, itemCount: 2, missingCount: 0, pendingCount: 0, errorCount: 0, blockingIssues: 0, lastScanErrorCode: "", lastScanCompleted: "", manifestStatus: "FILE_DIRTY", manifestCheckedAt: "2026-09-18T12:00:00Z" };
    const page = { page: 1, pageSize: 24, totalItems: 1, totalPages: 1, summary: { ...summary, all: 1 }, items: [row] };
    const pageMock: MockedResponse = { request: { query: MANAGE_GALLERIES, variables: { page: 1, issue: "ALL" } }, result: { data: { manageGalleries: page } } };
    render(<MemoryRouter initialEntries={["/manage/gallery"]}><MockedProvider mocks={[
      pageMock,
      { request: { query: PREVIEW_GALLERY_MANIFEST_BATCH_PUSH, variables: { setIDs: ["gallery-one"] } }, result: { data: { previewGalleryManifestPush: [{ setID: "gallery-one", title: "One", status: "FILE_DIRTY", metadataRevision: 4, path: "/media/one/.cosplay.json", fileHash: "hash", databaseContentChanged: true, localFileChanged: true, blockReason: "" }] } } },
      { request: { query: PUSH_GALLERY_MANIFESTS, variables: { items: [{ setID: "gallery-one", expectedMetadataRevision: 4, expectedPath: "/media/one/.cosplay.json", expectedFileHash: "hash" }], overwriteLocal: false } }, result: { data: { pushGalleryManifests: [{ setID: "gallery-one", outcome: "SKIPPED", reason: "LOCAL_FILE_CHANGED" }] } } },
      pageMock,
    ]}><ManageGalleryIndexPage /></MockedProvider></MemoryRouter>);
    fireEvent.click(await screen.findByRole("checkbox", { name: "Select One" }));
    fireEvent.click(screen.getByRole("button", { name: /Batch Push \(1\/100\)/ }));
    expect(await screen.findByRole("dialog", { name: "Batch Manifest Push preview" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /Skip changed files/ }));
    expect(await screen.findByRole("dialog", { name: "Batch Manifest Push result" })).toHaveTextContent("LOCAL_FILE_CHANGED");
  });

  it("explains a disabled metadata writeback policy before any Push can run", async () => {
    const row = { setID: "gallery-one", slug: "one", state: "ACTIVE", title: "One", contentRating: "NON_ADULT", metadataRevision: 4, scanRevision: 2, browsable: true, sourceType: "DIRECTORY", sourcePath: "/media/one", sourceAvailability: "AVAILABLE", reconcileState: "IN_SYNC", overLimit: false, itemCount: 2, missingCount: 0, pendingCount: 0, errorCount: 0, blockingIssues: 0, lastScanErrorCode: "", lastScanCompleted: "", manifestStatus: "NONE", manifestCheckedAt: "2026-09-18T12:00:00Z" };
    render(<MemoryRouter initialEntries={["/manage/gallery"]}><MockedProvider mocks={[
      { request: { query: MANAGE_GALLERIES, variables: { page: 1, issue: "ALL" } }, result: { data: { manageGalleries: { page: 1, pageSize: 24, totalItems: 1, totalPages: 1, summary: { ...summary, all: 1 }, items: [row] } } } },
      { request: { query: PREVIEW_GALLERY_MANIFEST_BATCH_PUSH, variables: { setIDs: ["gallery-one"] } }, result: { data: { previewGalleryManifestPush: [{ setID: "gallery-one", title: "One", status: "NONE", metadataRevision: 4, path: "/media/one/.cosplay.json", fileHash: "", databaseContentChanged: true, localFileChanged: false, blockReason: "METADATA_WRITEBACK_DISABLED" }] } } },
    ]}><ManageGalleryIndexPage /></MockedProvider></MemoryRouter>);
    fireEvent.click(await screen.findByRole("checkbox", { name: "Select One" }));
    fireEvent.click(screen.getByRole("button", { name: /Batch Push \(1\/100\)/ }));
    const dialog = await screen.findByRole("dialog", { name: "Batch Manifest Push preview" });
    expect(dialog).toHaveTextContent("元数据写回已关闭");
    expect(screen.getByRole("button", { name: /Skip changed files/ })).toBeDisabled();
    expect(screen.getByRole("button", { name: /Overwrite changed files/ })).toBeDisabled();
  });
});
