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
            coverResource: { itemUUID: "item-1", contentRevision: 2, profileHash: "profile", variant: "CARD_480", mimeType: "image/webp" } },
          { kind: "GALLERY", uuid: "g-2", slug: "alice-two", name: "Alice two", matchLevel: 3, coverResource: null },
        ],
        cosers: [{ kind: "COSER", uuid: "c-1", slug: "alice", name: "Alice", matchLevel: 1 }],
        works: [], characters: [], tags: [],
      } } },
    }]}><MemoryRouter><SearchPage /></MemoryRouter></MockedProvider></IntlProvider>);

    const input = screen.getByRole("textbox", { name: "搜索" });
    fireEvent.change(input, { target: { value: "Alice" } });
    fireEvent.submit(input.closest("form")!);
    const first = await screen.findByRole("link", { name: /Alice one/ });
    expect(first).toHaveAttribute("href", "/gallery/alice-one");
    expect(first.querySelector("img")).toHaveAttribute("src", "/resource/item/item-1/2/profile/CARD_480");
    expect(screen.getByRole("link", { name: /Alice two/ }).querySelector("img")).toBeNull();
    expect(screen.getByText("Alice", { selector: "strong" }).closest("a")).toHaveAttribute("href", "/coser/alice");
    expect(screen.getAllByRole("link").filter((link) => link.classList.contains("search-result"))).toHaveLength(3);
  });
});
