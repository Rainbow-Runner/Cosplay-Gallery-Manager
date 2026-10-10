import "@testing-library/jest-dom/vitest";

import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { messages } from "../i18n/messages";
import { clampCropTransform, cropSourceRect } from "./ImageCropperDialog";
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

function renderPanel(onUpdated = vi.fn(async () => undefined)) {
  render(<IntlProvider locale="en-GB" messages={messages["en-GB"]}>
    <ManageCoserAssetsPanel coser={coser} onUpdated={onUpdated} />
  </IntlProvider>);
  return onUpdated;
}

async function loadCropper(width = 1200, height = 800) {
  const dialog = await screen.findByRole("dialog");
  const image = dialog.querySelector("img")!;
  Object.defineProperty(image, "naturalWidth", { configurable: true, value: width });
  Object.defineProperty(image, "naturalHeight", { configurable: true, value: height });
  fireEvent.load(image);
  await waitFor(() => expect(within(dialog).getByRole("button", { name: "Confirm crop" })).toBeEnabled());
  return dialog;
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

beforeEach(() => {
  let objectURL = 0;
  Object.defineProperty(URL, "createObjectURL", { configurable: true, value: vi.fn(() => `blob:coser-preview-${++objectURL}`) });
  Object.defineProperty(URL, "revokeObjectURL", { configurable: true, value: vi.fn() });
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (this: HTMLElement) {
    const banner = this.classList?.contains("image-cropper__frame--banner");
    return { x: 0, y: 0, top: 0, left: 0, right: banner ? 900 : 600, bottom: 600, width: banner ? 900 : 600, height: banner ? 300 : 600, toJSON: () => ({}) };
  });
  vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue({ drawImage: vi.fn() } as unknown as CanvasRenderingContext2D);
  Object.defineProperty(HTMLCanvasElement.prototype, "toBlob", { configurable: true, value: vi.fn(function (callback: BlobCallback, type?: string) { callback(new Blob(["cropped"], { type })); }) });
});

describe("ManageCoserAssetsPanel", () => {
  it("opens a 1:1 cropper, zooms with the wheel, confirms, and uploads the cropped avatar", async () => {
    const onUpdated = vi.fn(async () => undefined);
    const fetchMock = vi.fn<typeof fetch>(async () => new Response("", { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    renderPanel(onUpdated);

    const original = new File([new Uint8Array([0xff, 0xd8, 0xff, 0xd9])], "avatar.jpg", { type: "image/jpeg" });
    fireEvent.change(screen.getAllByLabelText("Local image file")[0], { target: { files: [original] } });
    const dialog = await loadCropper();
    expect(within(dialog).getByRole("heading", { name: "Crop 1:1 avatar" })).toBeInTheDocument();
    const cropArea = within(dialog).getByRole("application", { name: "Interactive image crop area" });
    fireEvent.wheel(cropArea, { deltaY: -500 });
    expect(within(dialog).getByLabelText("Current zoom")).not.toHaveTextContent("1.00×");
    fireEvent.doubleClick(cropArea);
    expect(within(dialog).getByLabelText("Current zoom")).toHaveTextContent("1.00×");
    fireEvent.click(within(dialog).getByRole("button", { name: "Confirm crop" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(screen.getByText("Crop confirmed. Upload to save this managed image.")).toBeInTheDocument();
    fireEvent.submit(screen.getByRole("button", { name: "Upload avatar" }).closest("form")!);

    await waitFor(() => expect(onUpdated).toHaveBeenCalledOnce());
    const [url, options] = fetchMock.mock.calls[0]!;
    expect(url).toBe(`/manage/coser-assets/${coser.uuid}/avatar`);
    const body = options?.body as FormData;
    expect(body.get("crop_x")).toBe("0");
    expect(body.get("crop_y")).toBe("0");
    expect(body.get("crop_size")).toBe("1");
    expect(body.get("file")).toBeInstanceOf(File);
    expect((body.get("file") as File).name).toBe("avatar-cropped.jpg");
    expect(body.get("file")).not.toBe(original);
  });

  it("uses Enter to confirm a 3:1 Banner crop and uploads its centred result", async () => {
    const fetchMock = vi.fn<typeof fetch>(async () => new Response("", { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    renderPanel();

    const original = new File([new Uint8Array([0x89, 0x50, 0x4e, 0x47])], "banner.png", { type: "image/png" });
    fireEvent.change(screen.getAllByLabelText("Local image file")[1], { target: { files: [original] } });
    const dialog = await loadCropper(1600, 900);
    expect(within(dialog).getByRole("heading", { name: "Crop 3:1 Banner" })).toBeInTheDocument();
    fireEvent.keyDown(within(dialog).getByRole("application"), { key: "Enter" });
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    fireEvent.submit(screen.getByRole("button", { name: "Upload Banner" }).closest("form")!);

    await waitFor(() => expect(fetchMock).toHaveBeenCalledOnce());
    const body = fetchMock.mock.calls[0]![1]?.body as FormData;
    expect(body.get("focal_x")).toBe("0.5");
    expect(body.get("focal_y")).toBe("0.5");
    expect((body.get("file") as File).name).toBe("banner-cropped.png");
  });

  it("cancels the cropper with Escape without selecting an upload", async () => {
    const fetchMock = vi.fn<typeof fetch>();
    vi.stubGlobal("fetch", fetchMock);
    renderPanel();
    const original = new File(["image"], "avatar.webp", { type: "image/webp" });
    fireEvent.change(screen.getAllByLabelText("Local image file")[0], { target: { files: [original] } });
    const dialog = await screen.findByRole("dialog");
    fireEvent.keyDown(dialog, { key: "Escape" });
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Upload avatar" })).toBeDisabled();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("supports pointer dragging and two-pointer zooming", async () => {
    renderPanel();
    const original = new File(["image"], "avatar.jpg", { type: "image/jpeg" });
    fireEvent.change(screen.getAllByLabelText("Local image file")[0], { target: { files: [original] } });
    const dialog = await loadCropper();
    const cropArea = within(dialog).getByRole("application", { name: "Interactive image crop area" });
    const image = cropArea.querySelector("img")!;

    fireEvent.pointerDown(cropArea, { pointerId: 1, pointerType: "mouse", clientX: 300, clientY: 300 });
    fireEvent.pointerMove(cropArea, { pointerId: 1, pointerType: "mouse", clientX: 400, clientY: 300 });
    fireEvent.pointerUp(cropArea, { pointerId: 1, pointerType: "mouse", clientX: 400, clientY: 300 });
    expect(image.style.transform).toContain("translate(100px, 0px)");

    fireEvent.pointerDown(cropArea, { pointerId: 2, pointerType: "touch", clientX: 200, clientY: 300 });
    fireEvent.pointerDown(cropArea, { pointerId: 3, pointerType: "touch", clientX: 400, clientY: 300 });
    fireEvent.pointerMove(cropArea, { pointerId: 3, pointerType: "touch", clientX: 500, clientY: 300 });
    expect(within(dialog).getByLabelText("Current zoom")).toHaveTextContent("1.50×");
  });

  it("rejects a source image over 50 megapixels in the cropper", async () => {
    renderPanel();
    const original = new File(["image"], "huge.jpg", { type: "image/jpeg" });
    fireEvent.change(screen.getAllByLabelText("Local image file")[0], { target: { files: [original] } });
    const dialog = await screen.findByRole("dialog");
    const image = dialog.querySelector("img")!;
    Object.defineProperty(image, "naturalWidth", { configurable: true, value: 10_000 });
    Object.defineProperty(image, "naturalHeight", { configurable: true, value: 6_000 });
    fireEvent.load(image);
    expect(within(dialog).getByRole("alert")).toHaveTextContent("The selected image exceeds 50 megapixels.");
    expect(within(dialog).getByRole("button", { name: "Confirm crop" })).toBeDisabled();
  });

  it("rejects an unsupported browser file before opening the cropper", () => {
    const fetchMock = vi.fn<typeof fetch>();
    vi.stubGlobal("fetch", fetchMock);
    renderPanel();
    const file = new File(["svg"], "avatar.svg", { type: "image/svg+xml" });
    fireEvent.change(screen.getAllByLabelText("Local image file")[0], { target: { files: [file] } });
    expect(screen.getByRole("alert")).toHaveTextContent("Choose a JPEG, PNG, or static WebP file.");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("keeps crop transforms within 1x-3x and calculates the visible source region", () => {
    const metrics = { naturalWidth: 1200, naturalHeight: 800, frameWidth: 600, frameHeight: 600 };
    expect(clampCropTransform({ zoom: 8, x: 9999, y: -9999 }, metrics)).toEqual({ zoom: 3, x: 1050, y: -600 });
    expect(cropSourceRect(metrics, { zoom: 1, x: 0, y: 0 })).toEqual({ x: 200, y: 0, width: 800, height: 800 });
    expect(cropSourceRect(metrics, { zoom: 2, x: 300, y: -150 })).toEqual({ x: 200, y: 300, width: 400, height: 400 });
  });
});
