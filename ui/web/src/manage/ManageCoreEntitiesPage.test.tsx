import "@testing-library/jest-dom/vitest";

import type { MockedResponse } from "@apollo/client/testing";
import { MockedProvider } from "@apollo/client/testing/react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it } from "vitest";

import { MANAGE_CORE_ENTITIES, MANAGE_CORE_ENTITY, MANAGE_CORE_ENTITY_OPTIONS, MANAGE_COSER_NAME_CONFLICTS } from "../api/manage";
import { messages } from "../i18n/messages";
import { ManageCoreEntitiesPage, parseAliases, socialPlatformOptions } from "./ManageCoreEntitiesPage";
import type { ManageCoreEntity } from "./types";

afterEach(cleanup);

const emptyPage = { page: 1, pageSize: 30, totalItems: 0, totalPages: 0, items: [] };
const listVariables = (kind: "COSER" | "WORK" | "CHARACTER" | "TAG", overrides: Record<string, unknown> = {}) => ({
  kind, page: 1, pageSize: kind === "COSER" ? 30 : 60, query: "", coserAssetFilter: "ALL", ...overrides,
});

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
  it("requires review of exact Coser identity matches and can open the existing entity", async () => {
    const existing = {
      __typename: "ManageCoreEntity", kind: "COSER", uuid: "018f4c8e-7a9b-7def-8123-456789abcdea", name: "Alice", sortName: "", aliases: ["Alicia"], slug: "alice", metadataRevision: 3,
      workUUID: null, avatarURL: "/resource/coser/alice/3/avatar-480", bannerURL: null, avatarCrop: null, bannerFocalPoint: null,
      profileSummary: "Existing profile", biography: "", countryOrRegion: "", useInRecommendation: true, socialAccounts: [], parents: [],
    } as ManageCoreEntity & { __typename: string };
    renderPage([
      {
        request: { query: MANAGE_CORE_ENTITIES, variables: listVariables("COSER") },
        result: { data: { manageCoreEntities: { ...emptyPage, totalItems: 1, totalPages: 1, items: [existing] } } },
      },
      {
        request: { query: MANAGE_COSER_NAME_CONFLICTS, variables: { name: "Alice", limit: 10 } },
        result: { data: { manageCoserNameConflicts: [{ __typename: "ManageCoserNameConflict", coser: existing, matchedValues: ["Alice"], galleryCount: 4 }] } },
      },
      {
        request: { query: MANAGE_CORE_ENTITY, variables: { kind: "COSER", uuid: existing.uuid } },
        result: { data: { manageCoreEntity: existing } },
      },
    ], "/manage/cosers", true);

    const create = await screen.findByRole("button", { name: "Create" });
    fireEvent.change(screen.getByLabelText("Name (required)"), { target: { value: "Alice" } });
    expect(create).toBeDisabled();
    expect(await screen.findByText(/Found 1 exact identity match/)).toBeInTheDocument();
    expect(screen.getByText(/UUID …89abcdea · 4 Gallery/)).toBeInTheDocument();
    fireEvent.click(screen.getByLabelText(/confirm this is a different person/));
    expect(create).toBeEnabled();
    fireEvent.click(screen.getByRole("button", { name: "Open existing" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "Save revision 3" })).toBeInTheDocument());
    expect(screen.getByLabelText("Name (required)")).toHaveValue("Alice");
  });

  it("shows Coser avatars, blank placeholders, and Avatar/Banner completeness markers", async () => {
    const complete = {
      __typename: "ManageCoreEntity", kind: "COSER", uuid: "018f4c8e-7a9b-7def-8123-456789abcde1", name: "Alice", sortName: "", aliases: [], slug: "alice", metadataRevision: 2,
      workUUID: null, avatarURL: "/resource/coser/alice/2/avatar-480", bannerURL: "/resource/coser/alice/2/banner-1600", avatarCrop: null, bannerFocalPoint: null,
      profileSummary: "", biography: "", countryOrRegion: "", useInRecommendation: true, socialAccounts: [], parents: [],
    } as ManageCoreEntity & { __typename: string };
    const incomplete = { ...complete, uuid: "018f4c8e-7a9b-7def-8123-456789abcde2", name: "Bob", slug: "bob", avatarURL: null, bannerURL: null };
    renderPage([{
      request: { query: MANAGE_CORE_ENTITIES, variables: listVariables("COSER") },
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

  it("restores global search, completeness, page size, and page navigation from the URL", async () => {
    const coser = {
      __typename: "ManageCoreEntity", kind: "COSER", uuid: "018f4c8e-7a9b-7def-8123-456789abcde2", name: "Alice Portrait", sortName: "", aliases: ["Alicia"], slug: "alice-portrait", metadataRevision: 1,
      workUUID: null, avatarURL: "/resource/coser/alice-portrait/1/avatar-480", bannerURL: null, avatarCrop: null, bannerFocalPoint: null,
      profileSummary: "", biography: "", countryOrRegion: "", useInRecommendation: true, socialAccounts: [], parents: [],
    } as ManageCoreEntity & { __typename: string };
    renderPage([
      {
        request: { query: MANAGE_CORE_ENTITIES, variables: listVariables("COSER", { page: 2, pageSize: 60, query: "Alice", coserAssetFilter: "INCOMPLETE" }) },
        result: { data: { manageCoreEntities: { page: 2, pageSize: 60, totalItems: 125, totalPages: 3, items: [coser] } } },
      },
      {
        request: { query: MANAGE_CORE_ENTITIES, variables: listVariables("COSER", { page: 3, pageSize: 60, query: "Alice", coserAssetFilter: "INCOMPLETE" }) },
        result: { data: { manageCoreEntities: { page: 3, pageSize: 60, totalItems: 125, totalPages: 3, items: [coser] } } },
      },
    ], "/manage/cosers?q=Alice&assets=INCOMPLETE&pageSize=60&page=2", true);

    expect(await screen.findByRole("button", { name: /Alice Portrait/ })).toBeInTheDocument();
    expect(screen.getByLabelText("Search all Cosers")).toHaveValue("Alice");
    expect(screen.getByLabelText("Image completeness")).toHaveValue("INCOMPLETE");
    expect(screen.getByLabelText("Items per page")).toHaveValue("60");
    expect(screen.getByText("61–120 of 125")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Last page" }));
    expect(await screen.findByText("121–125 of 125")).toBeInTheDocument();
    expect(screen.getByLabelText("Page")).toHaveValue(3);
  });

  it("exposes search, Pinyin sort guidance, page size, and pagination for Works", async () => {
    const work = {
      __typename: "ManageCoreEntity", kind: "WORK", uuid: "018f4c8e-7a9b-7def-8123-456789abcdef", name: "Fate/Grand Order", sortName: "Fate Grand Order", aliases: ["FGO"], slug: "fate-grand-order", metadataRevision: 1,
      workUUID: null, avatarURL: null, bannerURL: null, avatarCrop: null, bannerFocalPoint: null,
      profileSummary: "", biography: "", countryOrRegion: "", useInRecommendation: true, socialAccounts: [], parents: [],
    } as ManageCoreEntity & { __typename: string };
    renderPage([{
      request: { query: MANAGE_CORE_ENTITIES, variables: listVariables("WORK", { page: 2, pageSize: 100, query: "Fate" }) },
      result: { data: { manageCoreEntities: { page: 2, pageSize: 100, totalItems: 150, totalPages: 2, items: [work] } } },
    }], "/manage/entities?kind=WORK&q=Fate&pageSize=100&page=2");

    expect(await screen.findByRole("button", { name: /Fate\/Grand Order/ })).toBeInTheDocument();
    expect(screen.getByLabelText("Search all Works")).toHaveValue("Fate");
    expect(screen.getByText(/Sorted by English name and Chinese Pinyin/)).toBeInTheDocument();
    expect(screen.getByLabelText("Items per page")).toHaveValue("100");
    expect(screen.getByText("101–150 of 150")).toBeInTheDocument();
    expect(screen.getByRole("navigation", { name: "Works list pagination" })).toBeInTheDocument();
  });

  it("edits Work Aliases as removable chips inside the input control", async () => {
    renderPage([{
      request: { query: MANAGE_CORE_ENTITIES, variables: listVariables("WORK") },
      result: { data: { manageCoreEntities: { ...emptyPage, pageSize: 60 } } },
    }], "/manage/entities?kind=WORK");

    const create = await screen.findByRole("button", { name: "Create" });
    fireEvent.change(screen.getByLabelText("Name (required)"), { target: { value: "Fate" } });
    const aliasInput = screen.getByLabelText("Aliases");
    fireEvent.change(aliasInput, { target: { value: "FGO" } });
    expect(create).toBeDisabled();
    expect(screen.getByText("Press Enter to add this Alias before saving.")).toBeInTheDocument();
    fireEvent.keyDown(aliasInput, { key: "Enter" });
    expect(aliasInput).toHaveValue("");
    expect(create).toBeEnabled();
    fireEvent.change(aliasInput, { target: { value: "Fate Series" } });
    fireEvent.keyDown(aliasInput, { key: "Enter" });

    fireEvent.click(screen.getByRole("button", { name: "Edit Alias FGO" }));
    expect(screen.queryByRole("button", { name: "Edit Alias FGO" })).not.toBeInTheDocument();
    expect(aliasInput).toHaveValue("FGO");
    fireEvent.change(aliasInput, { target: { value: "Fate Grand Order" } });
    fireEvent.click(screen.getByRole("button", { name: "Edit Alias Fate Series" }));
    expect(screen.getByRole("button", { name: "Edit Alias Fate Grand Order" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Edit Alias Fate Series" })).not.toBeInTheDocument();
    expect(aliasInput).toHaveValue("Fate Series");
    fireEvent.keyDown(aliasInput, { key: "Enter" });
    fireEvent.click(screen.getByRole("button", { name: "Remove Alias Fate Series" }));
    expect(screen.queryByRole("button", { name: "Edit Alias Fate Series" })).not.toBeInTheDocument();
    fireEvent.change(aliasInput, { target: { value: "Fate/stay night" } });
    fireEvent.keyDown(aliasInput, { key: "Enter" });
    expect(screen.getByRole("button", { name: "Edit Alias Fate/stay night" })).toBeInTheDocument();
  });

  it("keeps alias separators editable and parses aliases only for persistence", async () => {
    renderPage([{
      request: { query: MANAGE_CORE_ENTITIES, variables: listVariables("COSER") },
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
        request: { query: MANAGE_CORE_ENTITIES, variables: listVariables("CHARACTER") },
        result: { data: { manageCoreEntities: emptyPage } },
      },
      {
        request: { query: MANAGE_CORE_ENTITY_OPTIONS, variables: { kind: "WORK", query: "", limit: 20 } },
        result: { data: { manageCoreEntityOptions: [work] } },
      },
    ], "/manage/entities?kind=CHARACTER");

    const create = await screen.findByRole("button", { name: "Create" });
    expect(screen.getByPlaceholderText("Type an Alias and press Enter")).toBeInTheDocument();
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
        request: { query: MANAGE_CORE_ENTITIES, variables: listVariables("COSER") },
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
