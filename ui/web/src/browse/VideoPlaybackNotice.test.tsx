import { render, screen, cleanup } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { afterEach, describe, expect, it, vi } from "vitest";
import { messages } from "../i18n/messages";
import { VideoPlaybackNotice } from "./VideoPlaybackNotice";

afterEach(cleanup);
describe("archive playback explanations", () => {
  for (const locale of ["en-GB", "zh-CN"] as const) {
    for (const code of ["ARCHIVE_VIDEO_COMPRESSED", "ARCHIVE_VIDEO_DIRECT_UNAVAILABLE", "ARCHIVE_VIDEO_UNSAFE", "ARCHIVE_VIDEO_LIMIT", "ARCHIVE_VIDEO_SOURCE_CHANGED", "ARCHIVE_VIDEO_CODEC_UNSUPPORTED"]) {
      it(`${locale} ${code} explains refusal without offering conversion`, () => {
        render(<IntlProvider locale={locale} messages={messages[locale]}><VideoPlaybackNotice className="status" state={{ url: null, mode: "", preparing: false, failed: true, errorCode: code, retry: vi.fn(), onPlaybackError: vi.fn() }} /></IntlProvider>);
        expect(screen.getByRole("alert").textContent).toContain(messages[locale]["video.archiveManualExtract"]);
        expect(screen.getByRole("alert").textContent).toContain(code);
        expect(screen.queryByRole("button")).toBeNull();
      });
    }
  }
  for (const locale of ["en-GB", "zh-CN"] as const) {
    it(`${locale} explains a hardware failure without silently changing backend`, () => {
      render(<IntlProvider locale={locale} messages={messages[locale]}><VideoPlaybackNotice className="status" state={{ url: null, mode: "HLS_SESSION", preparing: false, failed: true, errorCode: "VIDEO_HARDWARE_HLS_FAILED", retry: vi.fn(), onPlaybackError: vi.fn() }} /></IntlProvider>);
      expect(screen.getByRole("alert").textContent).toContain(messages[locale]["video.hardwareFailed"]);
      expect(screen.getByRole("button")).not.toBeNull();
    });
  }
});
