import "@testing-library/jest-dom/vitest";

import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { messages } from "../i18n/messages";
import { avatarPreviewSourceRect, bannerPreviewSourceRect, ManageCoserAssetsPanel } from "./ManageCoserAssetsPanel";
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
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

beforeEach(() => {
  Object.defineProperty(URL, "createObjectURL", { configurable: true, value: vi.fn(() => "blob:coser-preview") });
  Object.defineProperty(URL, "revokeObjectURL", { configurable: true, value: vi.fn() });
  vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue({ clearRect: vi.fn(), drawImage: vi.fn() } as unknown as CanvasRenderingContext2D);
});

describe("ManageCoserAssetsPanel", () => {
  it("previews and uploads a browser avatar with normalized zoom and positioning", async () => {
    const onUpdated = vi.fn(async () => undefined);
    const fetchMock = vi.fn<typeof fetch>(async () => new Response("", { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    render(<IntlProvider locale="en-GB" messages={messages["en-GB"]}>
      <ManageCoserAssetsPanel coser={coser} onUpdated={onUpdated} />
    </IntlProvider>);

    const file = new File([new Uint8Array([0xff, 0xd8, 0xff, 0xd9])], "avatar.jpg", { type: "image/jpeg" });
    fireEvent.change(screen.getAllByLabelText("Local image file")[0], { target: { files: [file] } });
    await waitFor(() => expect(screen.getByLabelText("Avatar crop preview")).toBeInTheDocument());
    fireEvent.change(screen.getByLabelText("Avatar zoom"), { target: { value: "2" } });
    fireEvent.change(screen.getByLabelText("Avatar horizontal position"), { target: { value: "0.6" } });
    fireEvent.change(screen.getByLabelText("Avatar vertical position"), { target: { value: "0.7" } });
    const submit = screen.getByRole("button", { name: "Upload avatar" });
    fireEvent.submit(submit.closest("form")!);

    await waitFor(() => expect(onUpdated).toHaveBeenCalledOnce());
    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, options] = fetchMock.mock.calls[0]!;
    expect(url).toBe(`/manage/coser-assets/${coser.uuid}/avatar`);
    expect(options?.method).toBe("POST");
    const body = options?.body as FormData;
    expect(body.get("expected_metadata_revision")).toBe("6");
    expect(body.get("crop_x")).toBe("0.35");
    expect(body.get("crop_y")).toBe("0.45");
    expect(body.get("crop_size")).toBe("0.5");
    expect(body.get("file")).toBe(file);
    expect(URL.revokeObjectURL).toHaveBeenCalledWith("blob:coser-preview");
  });

  it("uploads a Banner using the focal point selected in its 3:1 preview", async () => {
    const fetchMock = vi.fn<typeof fetch>(async () => new Response("", { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    render(<IntlProvider locale="en-GB" messages={messages["en-GB"]}>
      <ManageCoserAssetsPanel coser={coser} onUpdated={async () => undefined} />
    </IntlProvider>);

    const file = new File([new Uint8Array([0x89, 0x50, 0x4e, 0x47])], "banner.png", { type: "image/png" });
    fireEvent.change(screen.getAllByLabelText("Local image file")[1], { target: { files: [file] } });
    await waitFor(() => expect(screen.getByLabelText("Banner crop preview")).toBeInTheDocument());
    fireEvent.change(screen.getByLabelText("Banner horizontal position"), { target: { value: "0.2" } });
    fireEvent.change(screen.getByLabelText("Banner vertical position"), { target: { value: "0.8" } });
    fireEvent.submit(screen.getByRole("button", { name: "Upload Banner" }).closest("form")!);

    await waitFor(() => expect(fetchMock).toHaveBeenCalledOnce());
    const body = fetchMock.mock.calls[0]![1]?.body as FormData;
    expect(body.get("focal_x")).toBe("0.2");
    expect(body.get("focal_y")).toBe("0.8");
    expect(body.get("file")).toBe(file);
  });

  it("rejects an unsupported browser file before upload", () => {
    const fetchMock = vi.fn<typeof fetch>();
    vi.stubGlobal("fetch", fetchMock);
    render(<IntlProvider locale="en-GB" messages={messages["en-GB"]}>
      <ManageCoserAssetsPanel coser={coser} onUpdated={async () => undefined} />
    </IntlProvider>);

    const file = new File(["svg"], "avatar.svg", { type: "image/svg+xml" });
    fireEvent.change(screen.getAllByLabelText("Local image file")[0], { target: { files: [file] } });
    expect(screen.getByRole("alert")).toHaveTextContent("Choose a JPEG, PNG, or static WebP file.");
    expect(screen.getByRole("button", { name: "Upload avatar" })).toBeDisabled();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("uses the same source rectangles as the server derivative framing", () => {
    expect(avatarPreviewSourceRect(1200, 800, { x: 0.25, y: 0.1, size: 0.5 })).toEqual({ x: 400, y: 80, width: 400, height: 400 });
    expect(bannerPreviewSourceRect(1200, 800, { x: 0.5, y: 0.75 })).toEqual({ x: 0, y: 400, width: 1200, height: 400 });
  });
});
