import "@testing-library/jest-dom/vitest";

import { render, screen } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";
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
  it.each([
    ["/", "browse-shell"],
    ["/manage", "manage-shell"],
    ["/setup", "setup-shell"],
  ])("renders %s in the expected shell", async (path, testID) => {
    renderRoute(path);
    expect(await screen.findByTestId(testID)).toBeInTheDocument();
  });
});
