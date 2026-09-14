import "@testing-library/jest-dom/vitest";

import type { MockedResponse } from "@apollo/client/testing";
import { MockedProvider } from "@apollo/client/testing/react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { afterEach, describe, expect, it } from "vitest";

import { MANAGE_GALLERIES } from "../api/manage";
import { ManageGalleryIndexPage } from "./ManageGalleryIndexPage";

afterEach(cleanup);

const summary = { all: 60, draft: 55, overLimit: 2, unavailable: 3, blocking: 4, missingGallery: 1, processingError: 2 };

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
  it("uses seven keyboard-accessible cards as the only filter and resets pagination", async () => {
    render(<MemoryRouter initialEntries={["/manage/gallery?page=3&issue=DRAFT"]}>
      <MockedProvider mocks={[galleryPage(3, "DRAFT"), galleryPage(1, "MISSING"), galleryPage(1, "PROCESSING_ERROR"), galleryPage(1, "ALL")]}>
        <><ManageGalleryIndexPage /><CurrentLocation /></>
      </MockedProvider>
    </MemoryRouter>);

    expect(screen.getByRole("heading", { name: "Gallery" })).toBeInTheDocument();
    const filters = await screen.findByRole("navigation", { name: "作品集筛选 / Gallery filters" });
    expect(filters.querySelectorAll("button")).toHaveLength(7);
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
});
