import "@testing-library/jest-dom/vitest";

import type { MockedResponse } from "@apollo/client/testing";
import { MockedProvider } from "@apollo/client/testing/react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it } from "vitest";

import { MANAGE_CORE_ENTITIES, MANAGE_CORE_ENTITY, MANAGE_CORE_ENTITY_OPTIONS } from "../api/manage";
import { messages } from "../i18n/messages";
import { ManageCoreEntitiesPage, parseAliases, socialPlatformOptions } from "./ManageCoreEntitiesPage";
import type { ManageCoreEntity } from "./types";

afterEach(cleanup);

const emptyPage = { page: 1, pageSize: 24, totalItems: 0, totalPages: 0, items: [] };

function renderPage(mocks: ReadonlyArray<MockedResponse>, initialEntry: string, coserOnly = false) {
  return render(<IntlProvider locale="en-GB" messages={messages["en-GB"]}>
    <MemoryRouter initialEntries={[initialEntry]}>
      <MockedProvider mocks={mocks}>
        <ManageCoreEntitiesPage coserOnly={coserOnly} />
      </MockedProvider>
    </MemoryRouter>
  </IntlProvider>);
}

describe("ManageCoreEntitiesPage validation", () => {
  it("shows Coser avatars, blank placeholders, and Avatar/Banner completeness markers", async () => {
    const complete = {
      __typename: "ManageCoreEntity", kind: "COSER", uuid: "018f4c8e-7a9b-7def-8123-456789abcde1", name: "Alice", sortName: "", aliases: [], slug: "alice", metadataRevision: 2,
      workUUID: null, avatarURL: "/resource/coser/alice/2/avatar-480", bannerURL: "/resource/coser/alice/2/banner-1600", avatarCrop: null, bannerFocalPoint: null,
      profileSummary: "", biography: "", countryOrRegion: "", useInRecommendation: true, socialAccounts: [], parents: [],
    } as ManageCoreEntity & { __typename: string };
    const incomplete = { ...complete, uuid: "018f4c8e-7a9b-7def-8123-456789abcde2", name: "Bob", slug: "bob", avatarURL: null, bannerURL: null };
    renderPage([{
      request: { query: MANAGE_CORE_ENTITIES, variables: { kind: "COSER", page: 1 } },
      result: { data: { manageCoreEntities: { ...emptyPage, totalItems: 2, totalPages: 1, items: [complete, incomplete] } } },
    }], "/manage/cosers", true);

    const alice = await screen.findByRole("button", { name: /Alice/ });
    const bob = screen.getByRole("button", { name: /Bob/ });
    expect(alice.querySelector(".entity-manage-list__avatar img")).toHaveAttribute("src", complete.avatarURL);
    expect(bob.querySelector(".entity-manage-list__avatar img")).not.toBeInTheDocument();
    expect(screen.getByLabelText("Alice: Avatar set")).toHaveClass("is-set");
    expect(screen.getByLabelText("Alice: Banner set")).toHaveClass("is-set");
    expect(screen.getByLabelText("Bob: Avatar missing")).not.toHaveClass("is-set");
    expect(screen.getByLabelText("Bob: Banner missing")).not.toHaveClass("is-set");
  });

  it("keeps alias separators editable and parses aliases only for persistence", async () => {
    renderPage([{
      request: { query: MANAGE_CORE_ENTITIES, variables: { kind: "COSER", page: 1 } },
      result: { data: { manageCoreEntities: emptyPage } },
    }], "/manage/cosers", true);

    const aliases = await screen.findByLabelText(/^Aliases/);
    fireEvent.change(aliases, { target: { value: "Komachi / こまち / 小 丁" } });
    expect(aliases).toHaveValue("Komachi / こまち / 小 丁");
    expect(parseAliases(String((aliases as HTMLInputElement).value))).toEqual(["Komachi", "こまち", "小 丁"]);
    expect(screen.getByText(/Separate multiple aliases with/)).toBeInTheDocument();
  });

  it("keeps Character creation disabled until both Name and Primary Work are set", async () => {
    const work = {
      kind: "WORK", uuid: "018f4c8e-7a9b-7def-8123-456789abcdef", name: "Fate",
      aliases: [], workUUID: null, metadataRevision: 1,
    };
    renderPage([
      {
        request: { query: MANAGE_CORE_ENTITIES, variables: { kind: "CHARACTER", page: 1 } },
        result: { data: { manageCoreEntities: emptyPage } },
      },
      {
        request: { query: MANAGE_CORE_ENTITY_OPTIONS, variables: { kind: "WORK", query: "", limit: 20 } },
        result: { data: { manageCoreEntityOptions: [work] } },
      },
    ], "/manage/entities?kind=CHARACTER");

    const create = await screen.findByRole("button", { name: "Create" });
    expect(create).toBeDisabled();
    fireEvent.change(screen.getByLabelText("Name (required)"), { target: { value: "Saber" } });
    expect(create).toBeDisabled();
    fireEvent.focus(screen.getByLabelText("Search Primary Work"));
    fireEvent.click(await screen.findByRole("option", { name: /Fate/ }));
    await waitFor(() => expect(create).toBeEnabled());
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  });

  it("offers common platform keys while retaining a valid custom key", async () => {
    const coser = {
      __typename: "ManageCoreEntity",
      kind: "COSER", uuid: "018f4c8e-7a9b-7def-8123-456789abcdea", name: "Alice",
      sortName: "", aliases: [], slug: "alice", metadataRevision: 1,
      workUUID: null, avatarURL: null, bannerURL: null, avatarCrop: null, bannerFocalPoint: null,
      profileSummary: "", biography: "", countryOrRegion: "", useInRecommendation: true,
      socialAccounts: [], parents: [],
    } as ManageCoreEntity & { __typename: string };
    renderPage([
      {
        request: { query: MANAGE_CORE_ENTITIES, variables: { kind: "COSER", page: 1 } },
        result: { data: { manageCoreEntities: { ...emptyPage, totalItems: 1, totalPages: 1, items: [coser] } } },
      },
      {
        request: { query: MANAGE_CORE_ENTITY, variables: { kind: "COSER", uuid: coser.uuid } },
        result: { data: { manageCoreEntity: coser } },
      },
    ], `/manage/cosers?uuid=${coser.uuid}`, true);

    fireEvent.click(await screen.findByRole("button", { name: "social" }));
    const platform = screen.getByLabelText(/^Platform key/);
    expect(platform).toHaveAttribute("list", "social-platform-keys");
    expect(document.querySelectorAll("#social-platform-keys option")).toHaveLength(socialPlatformOptions.length);
    const add = screen.getByRole("button", { name: "Add account" });
    expect(add).toBeDisabled();
    fireEvent.change(platform, { target: { value: "custom_site" } });
    fireEvent.change(screen.getByLabelText("HTTP(S) URL"), { target: { value: "https://example.test/profile" } });
    expect(add).toBeEnabled();
  });
});
