import { MockedProvider } from "@apollo/client/testing/react";
import { fireEvent, render, screen } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";

import { ENTITY_INDEX } from "../api/browse";
import { messages } from "../i18n/messages";
import { EntityIndexPage } from "./EntityIndexPage";

function page(query: string, items: Array<{ uuid: string; slug: string; name: string; aliases?: string[] }> = []) {
  return {
    request: { query: ENTITY_INDEX, variables: { kind: "COSER", scope: "ALL", page: 1, sort: "NAME", collectionType: "ALBUM", query } },
    result: { data: { entityIndex: { page: 1, pageSize: 30, totalItems: items.length, totalPages: 1,
      items: items.map((item) => ({ kind: "COSER", avatarURL: null, aliases: [], ...item })) } } },
  };
}

describe("EntityIndexPage person presentation", () => {
  it("uses the Album-filtered people query and Model detail route", async () => {
    render(
      <IntlProvider locale="en-GB" messages={messages["en-GB"]}>
        <MockedProvider mocks={[page("", [{ uuid: "alice-uuid", slug: "alice", name: "Alice", aliases: ["Alice Alias"] }]), page("Alice", [{ uuid: "alice-uuid", slug: "alice", name: "Alice", aliases: ["Alice Alias"] }])] }>
          <MemoryRouter>
            <EntityIndexPage kind="COSER" titleID="page.models" collectionType="ALBUM" routeName="model" fixedScope="ALL" searchable />
          </MemoryRouter>
        </MockedProvider>
      </IntlProvider>,
    );

    expect(await screen.findByRole("link", { name: "Alice" })).toHaveAttribute("href", "/model/alice");
    expect(screen.queryByText("Alice Alias")).not.toBeInTheDocument();
    const input = screen.getByRole("textbox", { name: "Search" });
    fireEvent.change(input, { target: { value: "Alice" } });
    fireEvent.submit(input.closest("form")!);
    expect(await screen.findByRole("link", { name: "Alice" })).toBeInTheDocument();
  });
});
