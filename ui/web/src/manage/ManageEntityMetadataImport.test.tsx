import "@testing-library/jest-dom/vitest";

import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { afterEach, describe, expect, it, vi } from "vitest";

import { messages } from "../i18n/messages";
import { ManageEntityMetadataImport } from "./ManageEntityMetadataImport";
import type { ManageCoreEntity } from "./types";

const work: ManageCoreEntity = {
  kind: "WORK", uuid: "2d9f6174-eaa9-45e6-8fc9-928e62b655aa", name: "鸣潮", sortName: "", aliases: ["Existing"], slug: "example",
  metadataRevision: 2, profileSummary: "", biography: "", countryOrRegion: "", useInRecommendation: false, socialAccounts: [], parents: [],
};

afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

describe("ManageEntityMetadataImport", () => {
  it("hides itself when the optional provider is removed", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ providers: [] }), { status: 200 })));
    const { container } = render(<IntlProvider locale="en-GB" messages={messages["en-GB"]}><ManageEntityMetadataImport entity={work} editorDirty={false} onUpdated={async () => undefined} /></IntlProvider>);
    await waitFor(() => expect(container.querySelector(".entity-metadata-import")).not.toBeInTheDocument());
  });

  it("reviews suggestions and applies only selected non-existing Aliases", async () => {
    const fetchMock = vi.fn<typeof fetch>(async (input) => {
      const url = String(input);
      if (url.endsWith("/providers")) return new Response(JSON.stringify({ providers: [{ key: "fixture", label: "Fixture" }] }), { status: 200 });
      if (url.endsWith("/search")) return new Response(JSON.stringify({ candidates: [{ ref: "article", display_name: "鸣潮", context: "Work", source_url: "https://example.test/article", match_quality: 100, type_confidence: "WORK" }] }), { status: 200 });
      if (url.endsWith("/prepare")) return new Response(JSON.stringify({ token: "token", provider_key: "fixture", kind: "WORK", display_name: "鸣潮", source_url: "https://example.test/article", page_id: "12", revision_id: "34", expires_at: "2026-09-03T12:15:00Z", suggestions: [
        { value: "Wuthering Waves", category: "official", language_hint: "latin", evidence: "官方译名", default_selected: true },
        { value: "Existing", category: "common", language_hint: "latin", evidence: "常用译名", default_selected: true },
        { value: "鳴潮", category: "original", language_hint: "", evidence: "原名", default_selected: false },
      ] }), { status: 200 });
      return new Response(JSON.stringify({ metadata_revision: 3 }), { status: 200 });
    });
    vi.stubGlobal("fetch", fetchMock); const onUpdated = vi.fn(async () => undefined);
    render(<IntlProvider locale="en-GB" messages={messages["en-GB"]}><ManageEntityMetadataImport entity={work} editorDirty={false} onUpdated={onUpdated} /></IntlProvider>);
    fireEvent.click(await screen.findByRole("button", { name: "Search candidates" }));
    fireEvent.click(await screen.findByRole("button", { name: /鸣潮/ }));
    const existing = await screen.findByRole("checkbox", { name: /Existing/ }); expect(existing).toBeDisabled();
    fireEvent.click(screen.getByRole("checkbox", { name: /鳴潮/ }));
    fireEvent.click(screen.getByRole("button", { name: "Apply selected Aliases" }));
    await waitFor(() => expect(onUpdated).toHaveBeenCalledOnce());
    const applyCall = fetchMock.mock.calls.find(([input]) => String(input).endsWith("/apply"));
    expect(JSON.parse(String(applyCall?.[1]?.body))).toEqual({ token: "token", expected_metadata_revision: 2, aliases: ["Wuthering Waves", "鳴潮"] });
  });

  it("shows an explicit empty result and blocks apply while local edits are dirty", async () => {
    const fetchMock = vi.fn<typeof fetch>(async (input) => {
      const url = String(input);
      if (url.endsWith("/providers")) return new Response(JSON.stringify({ providers: [{ key: "fixture", label: "Fixture" }] }), { status: 200 });
      if (url.endsWith("/search")) return new Response(JSON.stringify({ candidates: [] }), { status: 200 });
      return new Response("not found", { status: 404 });
    });
    vi.stubGlobal("fetch", fetchMock);
    render(<IntlProvider locale="en-GB" messages={messages["en-GB"]}><ManageEntityMetadataImport entity={work} editorDirty onUpdated={async () => undefined} /></IntlProvider>);
    expect(await screen.findByRole("status")).toHaveTextContent("Save or discard");
    fireEvent.change(screen.getByLabelText("Entity name"), { target: { value: "Unknown" } });
    fireEvent.click(screen.getByRole("button", { name: "Search candidates" }));
    expect(await screen.findByText(/No candidates matched “Unknown”/)).toBeInTheDocument();
  });
});
