import "@testing-library/jest-dom/vitest";

import type { MockedResponse } from "@apollo/client/testing";
import { MockedProvider } from "@apollo/client/testing/react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { afterEach, describe, expect, it } from "vitest";

import { MANAGE_GALLERIES, PREVIEW_GALLERY_MANIFEST_BATCH_PUSH, PUSH_GALLERY_MANIFESTS } from "../api/manage";
import { ManageGalleryIndexPage } from "./ManageGalleryIndexPage";

afterEach(cleanup);

const summary = { all: 60, draft: 55, overLimit: 2, unavailable: 3, blocking: 4, missingGallery: 1, processingError: 2, manifestAttention: 4, captureDateAttention: 1 };

function galleryPage(page: number, issue: string, search = ""): MockedResponse {
  return {
    request: { query: MANAGE_GALLERIES, variables: { page, issue, search } },
    result: { data: { manageGalleries: { page, pageSize: 24, totalItems: search ? 1 : issue === "ALL" ? 60 : issue === "MISSING" ? 1 : issue === "PROCESSING_ERROR" ? 2 : 55,
      totalPages: page === 3 ? 3 : 1, summary, items: [] } } },
  };
}

function CurrentLocation() {
  const location = useLocation();
  return <output data-testid="location">{location.search}</output>;
}

describe("ManageGalleryIndexPage filter cards", () => {
  it("uses nine keyboard-accessible cards as the only filter and resets pagination", async () => {
    render(<MemoryRouter initialEntries={["/manage/gallery?page=3&issue=DRAFT"]}>
      <MockedProvider mocks={[galleryPage(3, "DRAFT"), galleryPage(1, "MISSING"), galleryPage(1, "PROCESSING_ERROR"), galleryPage(1, "CAPTURE_DATE"), galleryPage(1, "ALL")]}>
        <><ManageGalleryIndexPage /><CurrentLocation /></>
      </MockedProvider>
    </MemoryRouter>);

    expect(screen.getByRole("heading", { name: "Gallery" })).toBeInTheDocument();
    const filters = await screen.findByRole("navigation", { name: "作品集筛选 / Gallery filters" });
    expect(filters.querySelectorAll("button")).toHaveLength(9);
    expect(screen.queryByRole("combobox")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /草稿 \/ Draft/ })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: /缺失媒体 \/ Missing/ })).toHaveTextContent("1");

    fireEvent.click(screen.getByRole("button", { name: /缺失媒体 \/ Missing/ }));
    expect(await screen.findByRole("button", { name: /缺失媒体 \/ Missing/ })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByTestId("location")).toHaveTextContent("?issue=MISSING");

    fireEvent.click(screen.getByRole("button", { name: /处理失败 \/ Processing error/ }));
    expect(await screen.findByRole("button", { name: /处理失败 \/ Processing error/ })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByTestId("location")).toHaveTextContent("?issue=PROCESSING_ERROR");

    fireEvent.click(screen.getByRole("button", { name: /拍摄时间待复核 \/ Date review/ }));
    expect(await screen.findByRole("button", { name: /拍摄时间待复核 \/ Date review/ })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByTestId("location")).toHaveTextContent("?issue=CAPTURE_DATE");

    fireEvent.click(screen.getByRole("button", { name: /全部 \/ All/ }));
    expect(await screen.findByRole("button", { name: /全部 \/ All/ })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByTestId("location").textContent).toBe("");
  });

  it("searches across pages, combines issue filters and preserves the query in the URL", async () => {
    render(<MemoryRouter initialEntries={["/manage/gallery?page=3&issue=DRAFT"]}>
      <MockedProvider mocks={[galleryPage(3, "DRAFT"), galleryPage(1, "DRAFT", "Alice"), galleryPage(1, "MISSING", "Alice"), galleryPage(1, "MISSING")]}>
        <><ManageGalleryIndexPage /><CurrentLocation /></>
      </MockedProvider>
    </MemoryRouter>);

    await screen.findByRole("navigation", { name: "作品集筛选 / Gallery filters" });
    fireEvent.change(screen.getByRole("searchbox", { name: /搜索作品集/ }), { target: { value: "Alice" } });
    await waitFor(() => expect(screen.getByTestId("location")).toHaveTextContent("issue=DRAFT&search=Alice"));
    expect(screen.getByTestId("location")).not.toHaveTextContent("page=3");
    expect(await screen.findByText(/1 个匹配作品集/)).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /缺失媒体 \/ Missing/ }));
    await waitFor(() => expect(screen.getByTestId("location")).toHaveTextContent("issue=MISSING&search=Alice"));
    fireEvent.click(screen.getByRole("button", { name: /清除 \/ Clear/ }));
    await waitFor(() => expect(screen.getByTestId("location")).toHaveTextContent("?issue=MISSING"));
  });

  it("reports when no Gallery matches the search", async () => {
    const emptyPage = { page: 1, pageSize: 24, totalItems: 0, totalPages: 0, summary, items: [] };
    render(<MemoryRouter initialEntries={["/manage/gallery?search=unknown"]}><MockedProvider mocks={[
      { request: { query: MANAGE_GALLERIES, variables: { page: 1, issue: "ALL", search: "unknown" } }, result: { data: { manageGalleries: emptyPage } } },
    ]}><ManageGalleryIndexPage /></MockedProvider></MemoryRouter>);
    expect(await screen.findByText(/没有匹配的作品集/)).toBeInTheDocument();
    expect(screen.getByRole("searchbox", { name: /搜索作品集/ })).toHaveValue("unknown");
  });

  it("previews a selected Gallery and offers explicit skip or overwrite decisions", async () => {
    const row = { setID: "gallery-one", slug: "one", state: "ACTIVE", title: "One", contentRating: "NON_ADULT", metadataRevision: 4, scanRevision: 2, browsable: true, sourceType: "DIRECTORY", sourcePath: "/media/one", sourceAvailability: "AVAILABLE", reconcileState: "IN_SYNC", overLimit: false, itemCount: 2, missingCount: 0, pendingCount: 0, errorCount: 0, blockingIssues: 0, lastScanErrorCode: "", lastScanCompleted: "", manifestStatus: "FILE_DIRTY", manifestCheckedAt: "2026-09-18T12:00:00Z", captureDateReviewStatus: "" };
    const page = { page: 1, pageSize: 24, totalItems: 1, totalPages: 1, summary: { ...summary, all: 1 }, items: [row] };
    const pageMock: MockedResponse = { request: { query: MANAGE_GALLERIES, variables: { page: 1, issue: "ALL", search: "" } }, result: { data: { manageGalleries: page } } };
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
    const row = { setID: "gallery-one", slug: "one", state: "ACTIVE", title: "One", contentRating: "NON_ADULT", metadataRevision: 4, scanRevision: 2, browsable: true, sourceType: "DIRECTORY", sourcePath: "/media/one", sourceAvailability: "AVAILABLE", reconcileState: "IN_SYNC", overLimit: false, itemCount: 2, missingCount: 0, pendingCount: 0, errorCount: 0, blockingIssues: 0, lastScanErrorCode: "", lastScanCompleted: "", manifestStatus: "NONE", manifestCheckedAt: "2026-09-18T12:00:00Z", captureDateReviewStatus: "" };
    render(<MemoryRouter initialEntries={["/manage/gallery"]}><MockedProvider mocks={[
      { request: { query: MANAGE_GALLERIES, variables: { page: 1, issue: "ALL", search: "" } }, result: { data: { manageGalleries: { page: 1, pageSize: 24, totalItems: 1, totalPages: 1, summary: { ...summary, all: 1 }, items: [row] } } } },
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
