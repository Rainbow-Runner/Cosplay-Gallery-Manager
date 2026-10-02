import { render, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const hls = vi.hoisted(() => ({ loadSource: vi.fn(), attachMedia: vi.fn(), destroy: vi.fn(), configuration: vi.fn() }));
vi.mock("hls.js", () => ({
  default: class {
    static Events = { ERROR: "hlsError" };
    static isSupported() { return true; }
    loadSource = hls.loadSource;
    attachMedia = hls.attachMedia;
    destroy = hls.destroy;
    on = vi.fn();
    constructor(configuration: unknown) { hls.configuration(configuration); }
  },
}));

import { PlaybackVideo } from "./PlaybackVideo";

describe("PlaybackVideo", () => {
  afterEach(() => { vi.restoreAllMocks(); hls.loadSource.mockClear(); hls.attachMedia.mockClear(); hls.destroy.mockClear(); hls.configuration.mockClear(); });

  it("attaches a local HLS client and destroys it when the viewer closes", async () => {
    vi.spyOn(HTMLMediaElement.prototype, "canPlayType").mockReturnValue("");
    vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(() => {});
    vi.spyOn(HTMLMediaElement.prototype, "load").mockImplementation(() => {});
    const url = "/playback/session/abc/index.m3u8";
    const { unmount } = render(<PlaybackVideo url={url} hls onError={vi.fn()} />);
    await waitFor(() => expect(hls.loadSource).toHaveBeenCalledWith(url));
    expect(hls.attachMedia).toHaveBeenCalledWith(expect.any(HTMLVideoElement));
    expect(hls.configuration).toHaveBeenCalledWith(expect.objectContaining({ maxBufferLength: 12,
      maxMaxBufferLength: 16, backBufferLength: 8, fragLoadingTimeOut: 30_000,
      fragLoadingMaxRetry: 2 }));
    unmount();
    expect(hls.destroy).toHaveBeenCalledOnce();
  });
});
