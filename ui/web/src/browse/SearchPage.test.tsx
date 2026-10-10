import "@testing-library/jest-dom/vitest";

import { MockedProvider } from "@apollo/client/testing/react";
import { fireEvent, render, screen } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";

import { SEARCH_PREVIEW } from "../api/browse";
import { messages } from "../i18n/messages";
import { SearchPage } from "./SearchPage";

describe("SearchPage results", () => {
  it("shows each Gallery as a full-width row with its cover and a safe fallback", async () => {
    render(<IntlProvider locale="zh-CN" messages={messages["zh-CN"]}><MockedProvider mocks={[{
      request: { query: SEARCH_PREVIEW, variables: { query: "Alice", scope: "LIST" } },
      result: { data: { searchPreview: {
        scope: "LIST", query: "Alice",
        galleries: [
          { kind: "GALLERY", uuid: "g-1", slug: "alice-one", name: "Alice one", matchLevel: 1,
            mediaAddedStartUTC: "2024-01-01T00:00:00Z", mediaAddedEndUTC: "2024-02-01T00:00:00Z", shootDate: "2024-03", shootDatePrecision: "MONTH", publishDate: "2025-04-05", publishDatePrecision: "DAY",
            coverResource: { itemUUID: "item-1", contentRevision: 2, profileHash: "profile", variant: "CARD_480", mimeType: "image/webp" } },
          { kind: "GALLERY", uuid: "g-2", slug: "alice-two", name: "Alice two", matchLevel: 3, mediaAddedStartUTC: "", mediaAddedEndUTC: "", shootDate: "", shootDatePrecision: "UNKNOWN", publishDate: "", publishDatePrecision: "UNKNOWN", coverResource: null },
        ],
        cosers: [{ kind: "COSER", uuid: "c-1", slug: "alice", name: "Alice", matchLevel: 1, mediaAddedStartUTC: "", mediaAddedEndUTC: "", shootDate: "", shootDatePrecision: "UNKNOWN", publishDate: "", publishDatePrecision: "UNKNOWN" }],
        works: [], characters: [], tags: [],
      } } },
    }]}><MemoryRouter><SearchPage /></MemoryRouter></MockedProvider></IntlProvider>);

    const input = screen.getByRole("textbox", { name: "搜索" });
    fireEvent.change(input, { target: { value: "Alice" } });
    fireEvent.submit(input.closest("form")!);
    const first = await screen.findByRole("link", { name: /Alice one/ });
    expect(first).toHaveAttribute("href", "/gallery/alice-one");
    expect(first.querySelector("img")).toHaveAttribute("src", "/resource/item/item-1/2/profile/CARD_480");
    expect(first).toHaveTextContent("2024-01-01 – 2024-02-01");
    expect(first).toHaveTextContent("2024-03");
    expect(first).toHaveTextContent("2025-04-05");
    expect(screen.getByRole("link", { name: /Alice two/ })).toHaveTextContent("--:--:--");
    expect(screen.getByRole("link", { name: /Alice two/ }).querySelector("img")).toBeNull();
    expect(screen.getByText("Alice", { selector: "strong" }).closest("a")).toHaveAttribute("href", "/coser/alice");
    expect(screen.getAllByRole("link").filter((link) => link.classList.contains("search-result"))).toHaveLength(3);
  });
});
