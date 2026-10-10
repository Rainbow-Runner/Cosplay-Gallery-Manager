import "@testing-library/jest-dom/vitest";

import type { MockedResponse } from "@apollo/client/testing";
import { MockedProvider } from "@apollo/client/testing/react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { MANAGE_RUNTIME_SETTINGS } from "../api/manage";
import { bytesToGiB, formatStorageBytes, gibToBytes, hardwareHelp, ManageSettingsPage } from "./ManageSettingsPage";

afterEach(() => { cleanup(); window.localStorage?.clear(); vi.unstubAllGlobals(); });

const runtimeSettings = {
  settingsRevision: 1, homeScope: "LIST", galleryCardScrubberEnabled: true, galleryDetailMediaFilterEnabled: true,
  galleryCardControlsVisible: true, mediaCardControlsVisible: true, detailPersonalControlsVisible: true,
  galleryAnimatedPlaybackLimit: 12, galleryAnimatedLockIntervalMS: 800,
  relatedLimit: 12, tagParentWeight: 0.5, tagMinimumScore: 0.2, tagMaximumDepth: 3,
  randomLimit: 24, randomStaticQuota: 0.7, randomGIFQuota: 0.15, randomVideoQuota: 0.15, randomGalleryRepeatDecay: 0.5,
  enhancedCacheMaximumBytes: 53687091200, minimumFreeBytes: 10737418240, minimumFreePercent: 0.05,
  automaticScanEnabled: false, automaticScanOnStartup: false, automaticScanIntervalMinutes: 1440, automaticSchedulesSuspended: false, dailyBackupEnabled: true, dailyBackupRetention: 7,
  archiveMaxEntries: 10000, archiveMaxEntryBytes: 2147483648, archiveMaxTotalBytes: 2147483648,
  archiveMaxCompressionRatio: 200, archiveMaxImagePixels: 250000000,
  videoHardwareMode: "SOFTWARE", videoHardwareFallbackEnabled: true, videoHardwareDevice: "",
};

describe("ManageSettingsPage cache status", () => {
  it("explains Docker device visibility, group permissions and driver failures separately", () => {
    expect(hardwareHelp("DEVICE_MISSING", "VAAPI")).toContain("这不代表宿主机没有核显");
    expect(hardwareHelp("PERMISSION_DENIED", "VAAPI")).toContain("group_add");
    expect(hardwareHelp("DRIVER_UNAVAILABLE", "VAAPI")).toContain("驱动加载失败");
    expect(hardwareHelp("SMOKE_TEST_FAILED", "VAAPI")).toContain("实际解码");
  });
  it("shows the cache path and occupied space as read-only deployment information", async () => {
    const mocks: MockedResponse[] = [{
      request: { query: MANAGE_RUNTIME_SETTINGS },
      result: { data: {
        manageRuntimeSettings: { __typename: "ManageRuntimeSettings", ...runtimeSettings },
        manageCacheStorage: { path: "/var/cache/cgm", byteSize: 156263337, fileCount: 216, baseByteSize: 3733456, enhancedByteSize: 152529881 },
		manageVideoDependencyStatus: {
			ffmpegAvailable: true, ffmpegSource: "PATH", ffmpegVersion: "7.1", ffmpegErrorCode: "",
			ffprobeAvailable: true, ffprobeSource: "FFMPEG_SIBLING", ffprobeVersion: "7.1", ffprobeErrorCode: "",
			hardwareProbeState: "COMPLETED", hardwareProbedAt: "2026-10-01T02:03:04Z",
			hardwareBackends: [
				{ backend: "NVENC", state: "AVAILABLE", device: "nvidia0", decodeCodecs: ["h264_cuvid", "hevc_cuvid"], encoder: "h264_nvenc", scaleFilter: "scale_cuda", runtimeTested: true, errorCode: "" },
				{ backend: "VAAPI", state: "PERMISSION_DENIED", device: "", decodeCodecs: ["h264", "hevc"], encoder: "h264_vaapi", scaleFilter: "scale_vaapi", runtimeTested: false, errorCode: "VAAPI_PERMISSION_DENIED" },
			],
		},
      } },
    }];
    const refreshed = structuredClone(mocks[0]);
    const refreshedResult = refreshed.result as { data: { manageVideoDependencyStatus: { hardwareProbedAt: string } } };
    refreshedResult.data.manageVideoDependencyStatus.hardwareProbedAt = "2026-10-08T02:03:04Z";
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, status: 202 });
    vi.stubGlobal("fetch", fetchMock);
    render(<MockedProvider mocks={[...mocks, refreshed]}><ManageSettingsPage /></MockedProvider>);

    expect(await screen.findByText("/var/cache/cgm")).toBeInTheDocument();
    expect(screen.getByText("149 MiB")).toBeInTheDocument();
    expect(screen.getByText("156,263,337 bytes · 216 files")).toBeInTheDocument();
		expect(screen.getByText("3.56 MiB")).toBeInTheDocument();
		expect(screen.getByText("145 MiB")).toBeInTheDocument();
		expect(screen.getByLabelText("可回收缓存上限（GiB）/ Reclaimable cache limit")).toHaveValue(50);
		expect(screen.getByLabelText("Animated playback limit")).toHaveValue(12);
		expect(screen.getByLabelText("Animation lock interval (ms)")).toHaveValue(800);
		expect(screen.getByText("可用 / Available")).toBeInTheDocument();
		expect(screen.getByText("权限不足 / Permission denied")).toBeInTheDocument();
		expect(screen.getByText("nvidia0 · h264_nvenc · scale_cuda · h264_cuvid / hevc_cuvid")).toBeInTheDocument();
		expect(screen.getByLabelText("转码模式 / Transcode mode")).toHaveValue("SOFTWARE");
		expect(screen.getByRole("option", { name: "NVENC" })).toBeEnabled();
		expect(screen.getByRole("option", { name: "VAAPI" })).toBeDisabled();
		fireEvent.change(screen.getByLabelText("转码模式 / Transcode mode"), { target: { value: "NVENC" } });
		expect(screen.getByLabelText("设备 / Device")).toBeEnabled();
		expect(screen.getByRole("option", { name: "nvidia0" })).toBeEnabled();
    fireEvent.click(screen.getByRole("button", { name: "重新检测硬件加速 / Detect hardware acceleration again" }));
    expect(await screen.findByText("检测完成 / Detection completed")).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledWith("/manage/video-hardware/probe", { method: "POST", credentials: "same-origin" });
    expect(screen.getByLabelText("转码模式 / Transcode mode")).toHaveValue("NVENC");
    expect(screen.getByLabelText("作者 / Author")).toBeChecked();
    expect(screen.getByLabelText("GPS 位置元数据")).not.toBeChecked();
    expect(screen.queryByDisplayValue("/var/cache/cgm")).not.toBeInTheDocument();
		fireEvent.change(screen.getByLabelText("Animation lock interval (ms)"), { target: { value: "699" } });
		expect(screen.getByRole("button", { name: "Save all runtime settings" })).toBeDisabled();
		fireEvent.click(screen.getByLabelText("启用自动扫描 / Enable automatic scans"));
		expect(screen.getByLabelText("服务启动后扫描一次 / Scan once after service startup")).not.toBeChecked();
		expect(screen.getByLabelText("扫描周期（分钟）/ Scan interval (minutes)")).toHaveValue(1440);
  });

  it("formats binary storage units without overstating precision", () => {
    expect(formatStorageBytes(0)).toBe("0 B");
    expect(formatStorageBytes(3733456)).toBe("3.56 MiB");
    expect(formatStorageBytes(152529881)).toBe("145 MiB");
  });

	it("converts the editable reclaimable limit without decimal byte drift", () => {
		expect(bytesToGiB(53687091200)).toBe(50);
		expect(gibToBytes("1.5")).toBe(1610612736);
	});
});
