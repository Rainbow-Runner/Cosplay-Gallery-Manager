import "@testing-library/jest-dom/vitest";

import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { afterEach, describe, expect, it, vi } from "vitest";

import { messages } from "../i18n/messages";
import { ManageCoserAssetsPanel } from "./ManageCoserAssetsPanel";
import type { ManageCoreEntity } from "./types";

const coser: ManageCoreEntity = {
  kind: "COSER",
  uuid: "2d9f6174-eaa9-45e6-8fc9-928e62b655aa",
  name: "Asset Coser",
  sortName: "",
  aliases: [],
  slug: "asset-coser",
  metadataRevision: 6,
  profileSummary: "",
  biography: "",
  countryOrRegion: "",
  useInRecommendation: false,
  avatarURL: "/resource/coser/2d9f6174-eaa9-45e6-8fc9-928e62b655aa/6/avatar-480",
  bannerURL: null,
  avatarCrop: { x: 0, y: 0, size: 1 },
  bannerFocalPoint: { x: 0.5, y: 0.5 },
  socialAccounts: [],
  parents: [],
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("ManageCoserAssetsPanel", () => {
  it("uploads one local image with revision and normalized framing", async () => {
    const onUpdated = vi.fn(async () => undefined);
    const fetchMock = vi.fn<typeof fetch>(async () => new Response("", { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    render(<IntlProvider locale="en-GB" messages={messages["en-GB"]}>
      <ManageCoserAssetsPanel coser={coser} onUpdated={onUpdated} />
    </IntlProvider>);

    const file = new File([new Uint8Array([0xff, 0xd8, 0xff, 0xd9])], "avatar.jpg", { type: "image/jpeg" });
    fireEvent.change(screen.getAllByLabelText("Local image file")[0], { target: { files: [file] } });
    fireEvent.change(screen.getByLabelText("Avatar crop X"), { target: { value: "0.1" } });
    fireEvent.change(screen.getByLabelText("Avatar crop Y"), { target: { value: "0.1" } });
    fireEvent.change(screen.getByLabelText("Avatar crop size"), { target: { value: "0.8" } });
    const submit = screen.getByRole("button", { name: "Upload avatar" });
    fireEvent.submit(submit.closest("form")!);

    await waitFor(() => expect(onUpdated).toHaveBeenCalledOnce());
    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, options] = fetchMock.mock.calls[0]!;
    expect(url).toBe(`/manage/coser-assets/${coser.uuid}/avatar`);
    expect(options?.method).toBe("POST");
    const body = options?.body as FormData;
    expect(body.get("expected_metadata_revision")).toBe("6");
    expect(body.get("crop_x")).toBe("0.1");
    expect(body.get("crop_y")).toBe("0.1");
    expect(body.get("crop_size")).toBe("0.8");
    expect(body.get("file")).toBe(file);
  });
});
