import { MockedProvider } from "@apollo/client/testing/react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { afterEach, describe, expect, it } from "vitest";

import { COSER_DETAIL } from "../api/browse";
import { messages } from "../i18n/messages";
import { CoserDetailPage, ModelDetailPage } from "./EntityDetailPages";

afterEach(cleanup);

const baseDetail = {
  entity: { kind: "COSER", uuid: "coser-alice", slug: "alice", name: "Alice", aliases: [], avatarURL: null },
  profileSummary: "",
  biography: "",
  countryOrRegion: "CN",
  bannerURL: null,
  socialAccounts: [
    {
      uuid: "social-1", platformKey: "twitter", label: "", handle: "@alice",
      url: "https://example.com/alice", status: "ACTIVE", position: "1024",
    },
    {
      uuid: "social-2", platformKey: "website", label: "Linktree", handle: "alice",
      url: "https://linktr.ee/alice", status: "ACTIVE", position: "2048",
    },
    {
      uuid: "social-3", platformKey: "custom_site", label: "Custom", handle: "alice",
      url: "https://example.com/custom", status: "INACTIVE", position: "3072",
    },
  ],
  redirected: false,
};

function LocationProbe() {
  const location = useLocation();
  return <output data-testid="location">{location.pathname}{location.search}</output>;
}

describe("CoserDetailPage", () => {
  it("uses the GalleryEpic-style identity composition and navigable numeric gallery pages", async () => {
    const mocks = [1, 2].map((page) => ({
      request: { query: COSER_DETAIL, variables: { slug: "alice", scope: "ALL", page, collectionType: "COSPLAY" } },
      result: { data: { coserDetail: {
        ...baseDetail,
        galleries: { page, pageSize: 24, totalItems: 120, totalPages: 5, items: [] },
      } } },
    }));

    render(
      <IntlProvider locale="en-GB" messages={messages["en-GB"]}>
        <MockedProvider mocks={mocks}>
          <MemoryRouter initialEntries={["/coser/alice"]}>
            <Routes><Route path="/coser/:slug" element={<CoserDetailPage />} /></Routes>
            <LocationProbe />
          </MemoryRouter>
        </MockedProvider>
      </IntlProvider>,
    );

    expect(await screen.findByRole("heading", { name: "Alice", level: 1 })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Manage Alice" })).toHaveAttribute("href", "/manage/cosers?uuid=coser-alice");
    expect(screen.getByRole("navigation", { name: "Breadcrumb" })).toHaveTextContent("HomeCosersAlice");
    expect(screen.getByRole("link", { name: "twitter: @alice" })).toHaveAttribute("href", "https://example.com/alice");
    expect(screen.getByRole("link", { name: "twitter: @alice" }).querySelector("svg.social-platform-icon.is-twitter")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "twitter: @alice" })).not.toHaveTextContent("X");
    expect(screen.getByRole("link", { name: "website: Linktree" }).querySelector("svg.social-platform-icon.is-linktree")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "custom_site: Custom" })).toHaveClass("is-inactive");
    const socialRow = document.querySelector(".social-accounts");
    expect(socialRow?.children).toHaveLength(7);
    expect(Array.from(socialRow?.children || []).slice(0, 6).map((element) => element.querySelector("svg")?.classList[1])).toEqual([
      "is-twitter", "is-facebook", "is-instagram", "is-weibo", "is-patreon", "is-linktree",
    ]);
    expect(socialRow?.querySelectorAll(".is-missing")).toHaveLength(4);
    expect(screen.queryByRole("link", { name: "Facebook" })).not.toBeInTheDocument();
    expect(document.querySelector(".coser-banner--empty")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "1" })).toHaveAttribute("aria-current", "page");

    fireEvent.click(screen.getByRole("button", { name: "2" }));
    await waitFor(() => expect(screen.getByTestId("location")).toHaveTextContent("/coser/alice?scope=ALL&page=2"));
    expect(await screen.findByRole("button", { name: "2" })).toHaveAttribute("aria-current", "page");
  });

  it("renders the same person identity as a Model with only Album galleries", async () => {
    const mock = {
      request: { query: COSER_DETAIL, variables: { slug: "alice", scope: "ALL", page: 1, collectionType: "ALBUM" } },
      result: { data: { coserDetail: {
        ...baseDetail,
        galleries: { page: 1, pageSize: 24, totalItems: 1, totalPages: 1, items: [] },
      } } },
    };

    render(
      <IntlProvider locale="en-GB" messages={messages["en-GB"]}>
        <MockedProvider mocks={[mock]}>
          <MemoryRouter initialEntries={["/model/alice"]}>
            <Routes><Route path="/model/:slug" element={<ModelDetailPage />} /></Routes>
          </MemoryRouter>
        </MockedProvider>
      </IntlProvider>,
    );

    expect(await screen.findByRole("heading", { name: "Alice", level: 1 })).toHaveClass("sr-only");
    expect(screen.getByRole("navigation", { name: "Breadcrumb" })).toHaveTextContent("HomeModelsAlice");
    expect(document.querySelector(".coser-profile")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "ALL" })).not.toBeInTheDocument();
  });
});
