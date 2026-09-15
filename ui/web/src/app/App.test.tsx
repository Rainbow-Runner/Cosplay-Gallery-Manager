import "@testing-library/jest-dom/vitest";

import { render, screen } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { MockedProvider } from "@apollo/client/testing/react";

import { messages } from "../i18n/messages";
import { HOME_GALLERIES } from "../api/browse";
import { AppRoutes } from "./App";

const mocks = [{
  request: { query: HOME_GALLERIES, variables: { page: 1 } },
  result: { data: { homeGalleries: { scope: "LIST", page: { page: 1, pageSize: 24, totalItems: 0, totalPages: 0, items: [] } } } },
}];

function renderRoute(path: string) {
  return render(
    <IntlProvider locale="en-GB" messages={messages["en-GB"]}>
      <MockedProvider mocks={mocks}>
        <MemoryRouter initialEntries={[path]}>
          <AppRoutes />
        </MemoryRouter>
      </MockedProvider>
    </IntlProvider>,
  );
}

describe("AppRoutes", () => {
  it("keeps English and Simplified Chinese message catalogues aligned", () => {
    expect(Object.keys(messages["zh-CN"]).sort()).toEqual(Object.keys(messages["en-GB"]).sort());
  });

  it.each([
    ["/", "browse-shell"],
    ["/manage", "manage-shell"],
    ["/setup", "setup-shell"],
  ])("renders %s in the expected shell", async (path, testID) => {
    renderRoute(path);
    expect(await screen.findByTestId(testID)).toBeInTheDocument();
  });

  it("renders public build, licence and corresponding source information", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ ok: true, json: async () => ({
      product: "Cosplay Gallery Manager", version: "1.0.0", gitHash: "0123abc",
      sourceCodeURL: "https://github.com/Rainbow-Runner/Cosplay-Gallery-Manager/tree/0123abc",
      exactSourceAvailable: true, license: "AGPL-3.0-or-later", warranty: "No warranty.", attribution: "Derived from Stash.",
    }) }));
    renderRoute("/legal");
    expect(await screen.findByText("AGPL-3.0-or-later")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Cosplay-Gallery-Manager\/tree\/0123abc/ })).toBeInTheDocument();
    vi.unstubAllGlobals();
  });

  it("routes the Manage navigation to the extensible Help page", async () => {
    renderRoute("/manage/help");
    expect(await screen.findByRole("heading", { name: "Help" })).toBeInTheDocument();
    const currentHelpLink = screen.getAllByRole("link", { name: "Help" }).find((link) => link.getAttribute("aria-current") === "page");
    expect(currentHelpLink).toHaveAttribute("href", "/manage/help");
  });
});
