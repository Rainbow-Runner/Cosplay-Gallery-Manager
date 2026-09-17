import "@testing-library/jest-dom/vitest";

import type { MockedResponse } from "@apollo/client/testing";
import { MockedProvider } from "@apollo/client/testing/react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it } from "vitest";

import { CREATE_CORE_ENTITY, MANAGE_CORE_ENTITIES, MANAGE_CORE_ENTITY, MANAGE_CORE_ENTITY_NAME_CONFLICTS, MANAGE_CORE_ENTITY_OPTIONS, MANAGE_COSER_NAME_CONFLICTS, MANAGE_TAG_TREE, MANAGE_WORK_CHARACTERS, PREVIEW_CORE_ENTITY_DELETE, UPDATE_CORE_ENTITY } from "../api/manage";
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
  it("defaults Tag management to the hierarchy and prepares a child under its selected parent", async () => {
    const parent = { uuid: "018f4c8e-7a9b-7def-8123-456789abcde0", name: "Costume", aliases: [], parentUUIDs: [], metadataRevision: 3, childCount: 0, galleryCount: 0 };
    renderPage([
      { request: { query: MANAGE_CORE_ENTITIES, variables: listVariables("TAG") }, result: { data: { manageCoreEntities: { ...emptyPage, pageSize: 60 } } } },
      { request: { query: MANAGE_TAG_TREE }, result: { data: { manageTagTree: [parent] } } },
    ], "/manage/entities?kind=TAG");
    fireEvent.click(await screen.findByRole("button", { name: "New child Tag under Costume" }));
    expect(screen.getByText("New child Tag under Costume")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create" })).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "Clear parent" }));
    expect(screen.queryByText("New child Tag under Costume")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "List" }));
    expect(screen.getByRole("combobox", { name: "Items per page" })).toBeInTheDocument();
  });
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
    await waitFor(() => expect(create).toBeEnabled());
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
    }, {
      request: { query: MANAGE_CORE_ENTITY_NAME_CONFLICTS, variables: { kind: "WORK", name: "Fate", limit: 10 } },
      result: { data: { manageCoreEntityNameConflicts: [] } },
    }], "/manage/entities?kind=WORK");

    const create = await screen.findByRole("button", { name: "Create" });
    fireEvent.change(screen.getByLabelText("Name (required)"), { target: { value: "Fate" } });
    const aliasInput = screen.getByLabelText("Aliases");
    fireEvent.change(aliasInput, { target: { value: "FGO" } });
    expect(create).toBeDisabled();
    expect(screen.getByText("Press Enter to add this Alias before saving.")).toBeInTheDocument();
    fireEvent.keyDown(aliasInput, { key: "Enter" });
    expect(aliasInput).toHaveValue("");
    await waitFor(() => expect(create).toBeEnabled());
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
      {
        request: { query: MANAGE_CORE_ENTITY_NAME_CONFLICTS, variables: { kind: "CHARACTER", name: "Saber", limit: 10 } },
        result: { data: { manageCoreEntityNameConflicts: [] } },
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
    expect(screen.getByText("Fate")).toBeInTheDocument();
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  });

  it("restores the Primary Work name when an existing Character is opened", async () => {
    const character = {
      __typename: "ManageCoreEntity", kind: "CHARACTER", uuid: "018f4c8e-7a9b-7def-8123-456789abcd44", name: "Saber", sortName: "", aliases: ["Artoria"], slug: "saber", metadataRevision: 2,
      workUUID: "018f4c8e-7a9b-7def-8123-456789abcd22", workName: "Fate", avatarURL: null, bannerURL: null, avatarCrop: null, bannerFocalPoint: null,
      profileSummary: "", biography: "", countryOrRegion: "", useInRecommendation: true, socialAccounts: [], parents: [],
    } as ManageCoreEntity & { __typename: string };
    renderPage([{
      request: { query: MANAGE_CORE_ENTITIES, variables: listVariables("CHARACTER") },
      result: { data: { manageCoreEntities: { ...emptyPage, pageSize: 60, totalItems: 1, totalPages: 1, items: [character] } } },
    }, {
      request: { query: MANAGE_CORE_ENTITY, variables: { kind: "CHARACTER", uuid: character.uuid } },
      result: { data: { manageCoreEntity: character } },
    }], `/manage/entities?kind=CHARACTER&uuid=${character.uuid}`);

    const listedWork = await screen.findByText("Fate", { selector: ".entity-manage-list__work" });
    expect(listedWork).toBeInTheDocument();
    expect(listedWork.closest("button")).toHaveClass("is-character");
    expect(screen.queryByText("Work: Fate")).not.toBeInTheDocument();
    await screen.findByRole("button", { name: "Save revision 2" });
    expect(screen.getByText("Fate", { selector: ".manage-entity-selector > span" })).toBeInTheDocument();
    expect(screen.getByLabelText("Primary Work UUID")).toHaveValue(character.workUUID);
  });

  it("requires explicit confirmation before moving an existing Character to another Work", async () => {
    const character = {
      __typename: "ManageCoreEntity", kind: "CHARACTER", uuid: "018f4c8e-7a9b-7def-8123-456789abcd44", name: "Saber", sortName: "", aliases: ["Artoria"], slug: "saber", metadataRevision: 2,
      workUUID: "018f4c8e-7a9b-7def-8123-456789abcd22", workName: "Fate", avatarURL: null, bannerURL: null, avatarCrop: null, bannerFocalPoint: null,
      profileSummary: "", biography: "", countryOrRegion: "", useInRecommendation: true, socialAccounts: [], parents: [],
    } as ManageCoreEntity & { __typename: string };
    const targetWork = { kind: "WORK", uuid: "018f4c8e-7a9b-7def-8123-456789abcd55", name: "Fate/Grand Order", aliases: ["FGO"], workUUID: null, workName: "", metadataRevision: 4 };
    const moved = { ...character, metadataRevision: 3, workUUID: targetWork.uuid, workName: targetWork.name };
    const updateInput = { kind: "CHARACTER", name: "Saber", sortName: "", aliases: ["Artoria"], workUUID: targetWork.uuid, profileSummary: "", biography: "", countryOrRegion: "", useInRecommendation: true };
    renderPage([
      { request: { query: MANAGE_CORE_ENTITIES, variables: listVariables("CHARACTER") }, result: { data: { manageCoreEntities: { ...emptyPage, pageSize: 60, totalItems: 1, totalPages: 1, items: [character] } } } },
      { request: { query: MANAGE_CORE_ENTITY, variables: { kind: "CHARACTER", uuid: character.uuid } }, result: { data: { manageCoreEntity: character } } },
      { request: { query: PREVIEW_CORE_ENTITY_DELETE, variables: { kind: "CHARACTER", uuid: character.uuid } }, result: { data: { previewCoreEntityDelete: { kind: "CHARACTER", uuid: character.uuid, metadataRevision: 2, referenceCount: 0, blockers: [], canDelete: true } } } },
      { request: { query: MANAGE_CORE_ENTITY_OPTIONS, variables: { kind: "WORK", query: "", limit: 20 } }, result: { data: { manageCoreEntityOptions: [targetWork] } } },
      { request: { query: UPDATE_CORE_ENTITY, variables: { uuid: character.uuid, expectedMetadataRevision: 2, input: updateInput } }, result: { data: { updateCoreEntity: moved } } },
      { request: { query: MANAGE_CORE_ENTITIES, variables: listVariables("CHARACTER") }, result: { data: { manageCoreEntities: { ...emptyPage, pageSize: 60, totalItems: 1, totalPages: 1, items: [moved] } } } },
    ], `/manage/entities?kind=CHARACTER&uuid=${character.uuid}`);

    await screen.findByRole("button", { name: "Save revision 2" });
    fireEvent.focus(screen.getByLabelText("Search Primary Work"));
    fireEvent.click(await screen.findByRole("option", { name: /Fate\/Grand Order/ }));
    fireEvent.click(screen.getByRole("button", { name: "Save revision 2" }));
    expect(await screen.findByRole("heading", { name: "Move Saber to another Work?" })).toBeInTheDocument();
    expect(screen.getByText("Primary Work will change: Fate → Fate/Grand Order.")).toBeInTheDocument();
    const commit = screen.getByRole("button", { name: "Move Character and save" });
    expect(commit).toBeDisabled();
    fireEvent.change(screen.getByLabelText("Type MOVE to continue"), { target: { value: "MOVE" } });
    fireEvent.click(commit);
    expect(await screen.findByText("Saved")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save revision 3" })).toBeInTheDocument();
  });

  it("creates and edits Characters inside a Work with the Work association locked", async () => {
    const work = {
      __typename: "ManageCoreEntity", kind: "WORK", uuid: "018f4c8e-7a9b-7def-8123-456789abcd22", name: "Fate", sortName: "", aliases: [], slug: "fate", metadataRevision: 2,
      workUUID: null, workName: "", avatarURL: null, bannerURL: null, avatarCrop: null, bannerFocalPoint: null,
      profileSummary: "", biography: "", countryOrRegion: "", useInRecommendation: true, socialAccounts: [], parents: [],
    } as ManageCoreEntity & { __typename: string };
    const saber = {
      ...work, kind: "CHARACTER", uuid: "018f4c8e-7a9b-7def-8123-456789abcd33", name: "Saber", slug: "saber", metadataRevision: 1,
      workUUID: work.uuid, workName: work.name,
    } as ManageCoreEntity & { __typename: string };
    const rin = {
      ...saber, uuid: "018f4c8e-7a9b-7def-8123-456789abcd44", name: "Rin", slug: "rin",
    } as ManageCoreEntity & { __typename: string };
    const characterInput = { kind: "CHARACTER", name: "Rin", sortName: "", aliases: [], workUUID: work.uuid, profileSummary: "", biography: "", countryOrRegion: "", useInRecommendation: true };
    renderPage([
      {
        request: { query: MANAGE_CORE_ENTITIES, variables: listVariables("WORK") },
        result: { data: { manageCoreEntities: { ...emptyPage, pageSize: 60, totalItems: 1, totalPages: 1, items: [work] } } },
      },
      { request: { query: MANAGE_CORE_ENTITY, variables: { kind: "WORK", uuid: work.uuid } }, result: { data: { manageCoreEntity: work } } },
      { request: { query: PREVIEW_CORE_ENTITY_DELETE, variables: { kind: "WORK", uuid: work.uuid } }, result: { data: { previewCoreEntityDelete: { kind: "WORK", uuid: work.uuid, metadataRevision: 2, referenceCount: 1, blockers: [{ code: "CHARACTER_REFERENCE", referenceCount: 1 }], canDelete: false } } } },
      { request: { query: MANAGE_WORK_CHARACTERS, variables: { workUUID: work.uuid } }, result: { data: { manageWorkCharacters: [saber] } } },
      { request: { query: MANAGE_CORE_ENTITY_NAME_CONFLICTS, variables: { kind: "CHARACTER", name: "Rin", limit: 10 } }, result: { data: { manageCoreEntityNameConflicts: [] } } },
      { request: { query: CREATE_CORE_ENTITY, variables: { input: characterInput } }, result: { data: { createCoreEntity: rin } } },
      { request: { query: MANAGE_WORK_CHARACTERS, variables: { workUUID: work.uuid } }, result: { data: { manageWorkCharacters: [saber, rin] } } },
      { request: { query: PREVIEW_CORE_ENTITY_DELETE, variables: { kind: "CHARACTER", uuid: rin.uuid } }, result: { data: { previewCoreEntityDelete: { kind: "CHARACTER", uuid: rin.uuid, metadataRevision: 1, referenceCount: 0, blockers: [], canDelete: true } } } },
    ], `/manage/entities?kind=WORK&uuid=${work.uuid}`);

    fireEvent.click(await screen.findByRole("button", { name: "Characters" }));
    expect(screen.getByRole("heading", { name: "Fate" })).toBeInTheDocument();
    expect(await screen.findByRole("button", { name: "Saber" })).toBeInTheDocument();
    expect(screen.getByText("1 total")).toBeInTheDocument();
    expect(screen.queryByLabelText("Search Primary Work")).not.toBeInTheDocument();
    expect(screen.getByText("Fate", { selector: ".fixed-character-work" })).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("Name (required)"), { target: { value: "Rin" } });
    await waitFor(() => expect(screen.getByRole("button", { name: "Create" })).toBeEnabled());
    fireEvent.click(screen.getByRole("button", { name: "Create" }));
    expect(await screen.findByText("Character saved and associated with this Work.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save revision 1" })).toBeInTheDocument();
  });

  it("requires review of exact Work matches before creating another Work", async () => {
    const existing = {
      __typename: "ManageCoreEntity", kind: "WORK", uuid: "018f4c8e-7a9b-7def-8123-456789abcd11", name: "Fate/stay night", sortName: "", aliases: ["Fate SN"], slug: "fate-stay-night", metadataRevision: 2,
      workUUID: null, avatarURL: null, bannerURL: null, avatarCrop: null, bannerFocalPoint: null,
      profileSummary: "", biography: "", countryOrRegion: "", useInRecommendation: true, socialAccounts: [], parents: [],
    } as ManageCoreEntity & { __typename: string };
    renderPage([{
      request: { query: MANAGE_CORE_ENTITIES, variables: listVariables("WORK") },
      result: { data: { manageCoreEntities: { ...emptyPage, pageSize: 60, totalItems: 1, totalPages: 1, items: [existing] } } },
    }, {
      request: { query: MANAGE_CORE_ENTITY_NAME_CONFLICTS, variables: { kind: "WORK", name: "Fate SN", limit: 10 } },
      result: { data: { manageCoreEntityNameConflicts: [{ __typename: "ManageCoreEntityNameConflict", entity: existing, matchedValues: ["Fate SN"], galleryCount: 3, workName: "", primaryNameMatch: false }] } },
    }], "/manage/entities?kind=WORK");

    const create = await screen.findByRole("button", { name: "Create" });
    fireEvent.change(screen.getByLabelText("Name (required)"), { target: { value: "Fate SN" } });
    expect(create).toBeDisabled();
    expect(await screen.findByText(/Found 1 exact match/)).toBeInTheDocument();
    expect(screen.getByText(/UUID …89abcd11 · 3 Gallery/)).toBeInTheDocument();
    fireEvent.click(screen.getByLabelText(/confirm this is a different Work/));
    expect(create).toBeEnabled();
  });

  it("blocks a duplicate Character primary Name inside the selected Work", async () => {
    const work = { kind: "WORK", uuid: "018f4c8e-7a9b-7def-8123-456789abcd22", name: "Fate", aliases: [], workUUID: null, metadataRevision: 1 };
    const existing = {
      __typename: "ManageCoreEntity", kind: "CHARACTER", uuid: "018f4c8e-7a9b-7def-8123-456789abcd33", name: "Saber", sortName: "", aliases: ["Artoria"], slug: "saber", metadataRevision: 1,
      workUUID: work.uuid, avatarURL: null, bannerURL: null, avatarCrop: null, bannerFocalPoint: null,
      profileSummary: "", biography: "", countryOrRegion: "", useInRecommendation: true, socialAccounts: [], parents: [],
    } as ManageCoreEntity & { __typename: string };
    renderPage([{
      request: { query: MANAGE_CORE_ENTITIES, variables: listVariables("CHARACTER") },
      result: { data: { manageCoreEntities: emptyPage } },
    }, {
      request: { query: MANAGE_CORE_ENTITY_OPTIONS, variables: { kind: "WORK", query: "", limit: 20 } },
      result: { data: { manageCoreEntityOptions: [work] } },
    }, {
      request: { query: MANAGE_CORE_ENTITY_NAME_CONFLICTS, variables: { kind: "CHARACTER", name: "Saber", limit: 10 } },
      result: { data: { manageCoreEntityNameConflicts: [{ __typename: "ManageCoreEntityNameConflict", entity: existing, matchedValues: ["Saber"], galleryCount: 2, workName: "Fate", primaryNameMatch: true }] } },
    }], "/manage/entities?kind=CHARACTER");

    const create = await screen.findByRole("button", { name: "Create" });
    fireEvent.change(screen.getByLabelText("Name (required)"), { target: { value: "Saber" } });
    expect(await screen.findByText("Primary Work: Fate")).toBeInTheDocument();
    fireEvent.focus(screen.getByLabelText("Search Primary Work"));
    fireEvent.click(await screen.findByRole("option", { name: /Fate/ }));
    expect(await screen.findByText(/already exists in the selected Work/)).toBeInTheDocument();
    expect(create).toBeDisabled();
    expect(screen.queryByLabelText(/confirm this is a different Character/)).not.toBeInTheDocument();
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
