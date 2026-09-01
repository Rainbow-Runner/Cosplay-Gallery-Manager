import "@testing-library/jest-dom/vitest";

import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { afterEach, describe, expect, it, vi } from "vitest";

import { messages } from "../i18n/messages";
import { ManageCoserMetadataImport } from "./ManageCoserMetadataImport";
import type { ManageCoreEntity } from "./types";

const coser: ManageCoreEntity = {
  kind: "COSER", uuid: "2d9f6174-eaa9-45e6-8fc9-928e62b655aa", name: "Example", sortName: "", aliases: [], slug: "example",
  metadataRevision: 2, profileSummary: "", biography: "", countryOrRegion: "", useInRecommendation: false,
  avatarURL: null, bannerURL: null, socialAccounts: [{ uuid: "account", platformKey: "twitter", label: "X", handle: "saved", url: "https://twitter.com/saved", status: "ACTIVE", visible: true, position: "1024" }], parents: [],
};

afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

describe("ManageCoserMetadataImport", () => {
  it("hides itself when the core build has no provider", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ providers: [] }), { status: 200, headers: { "Content-Type": "application/json" } })));
    const { container } = render(<IntlProvider locale="en-GB" messages={messages["en-GB"]}><ManageCoserMetadataImport coser={coser} onUpdated={async () => undefined} /></IntlProvider>);
    await waitFor(() => expect(container.querySelector(".coser-metadata-import")).not.toBeInTheDocument());
  });

  it("requires candidate review and sends only selected metadata", async () => {
    const fetchMock = vi.fn<typeof fetch>(async (input) => {
      const url = String(input);
      if (url.endsWith("/providers")) return new Response(JSON.stringify({ providers: [{ key: "fixture", label: "Fixture" }] }), { status: 200 });
      if (url.endsWith("/search")) return new Response(JSON.stringify({ candidates: [{ ref: "7", display_name: "Example", source_url: "https://example.test/7", match_quality: 100 }] }), { status: 200 });
      if (url.endsWith("/prepare")) return new Response(JSON.stringify({ token: "token", provider_key: "fixture", display_name: "Example", source_url: "https://example.test/7", has_avatar: true, has_banner: false, expires_at: "2026-08-12T12:15:00Z", accounts: [{ platform_key: "twitter", label: "X", handle: "saved", url: "https://twitter.com/saved" }, { platform_key: "website", label: "Website", handle: "new", url: "https://example.test/new" }] }), { status: 200 });
      return new Response(JSON.stringify({ metadata_revision: 4 }), { status: 200 });
    });
    vi.stubGlobal("fetch", fetchMock); const onUpdated = vi.fn(async () => undefined);
    render(<IntlProvider locale="en-GB" messages={messages["en-GB"]}><ManageCoserMetadataImport coser={coser} onUpdated={onUpdated} /></IntlProvider>);
    await screen.findByRole("button", { name: "Search candidates" }); fireEvent.click(screen.getByRole("button", { name: "Search candidates" }));
    fireEvent.click(await screen.findByRole("button", { name: /Example/ }));
    const apply = await screen.findByRole("button", { name: "Apply selected metadata" }); expect(apply).toBeEnabled(); fireEvent.click(apply);
    await waitFor(() => expect(onUpdated).toHaveBeenCalledOnce());
    const applyCall = fetchMock.mock.calls.find(([input]) => String(input).endsWith("/apply"));
    expect(JSON.parse(String(applyCall?.[1]?.body))).toMatchObject({ token: "token", expected_metadata_revision: 2, import_avatar: true, account_urls: ["https://example.test/new"] });
  });

  it("treats a legacy null account collection as empty without blanking the page", async () => {
    const fetchMock = vi.fn<typeof fetch>(async (input) => {
      const url = String(input);
      if (url.endsWith("/providers")) return new Response(JSON.stringify({ providers: [{ key: "fixture", label: "Fixture" }] }), { status: 200 });
      if (url.endsWith("/search")) return new Response(JSON.stringify({ candidates: [{ ref: "8", display_name: "No Accounts", source_url: "https://example.test/8", match_quality: 100 }] }), { status: 200 });
      if (url.endsWith("/prepare")) return new Response(JSON.stringify({ token: "empty-token", provider_key: "fixture", display_name: "No Accounts", source_url: "https://example.test/8", has_avatar: true, has_banner: true, expires_at: "2026-09-01T14:00:00Z", accounts: null }), { status: 200 });
      return new Response("not found", { status: 404 });
    });
    vi.stubGlobal("fetch", fetchMock);
    render(<IntlProvider locale="en-GB" messages={messages["en-GB"]}><ManageCoserMetadataImport coser={coser} onUpdated={async () => undefined} /></IntlProvider>);
    fireEvent.click(await screen.findByRole("button", { name: "Search candidates" }));
    fireEvent.click(await screen.findByRole("button", { name: /No Accounts/ }));
    expect(await screen.findByRole("button", { name: "Apply selected metadata" })).toBeEnabled();
    expect(screen.queryByText(/could not display this candidate/)).not.toBeInTheDocument();
    expect(document.querySelectorAll(".coser-metadata-accounts label")).toHaveLength(0);
  });

  it("contains an unexpected metadata panel render failure", async () => {
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => undefined);
    const malformed = { ...coser, socialAccounts: null } as unknown as ManageCoreEntity;
    render(<IntlProvider locale="en-GB" messages={messages["en-GB"]}><ManageCoserMetadataImport coser={malformed} onUpdated={async () => undefined} /></IntlProvider>);
    expect(await screen.findByRole("alert")).toHaveTextContent(/rest of the Coser editor remains available/);
    expect(screen.getByRole("button", { name: "Retry metadata panel" })).toBeInTheDocument();
    expect(consoleError).toHaveBeenCalled();
    consoleError.mockRestore();
  });
});
