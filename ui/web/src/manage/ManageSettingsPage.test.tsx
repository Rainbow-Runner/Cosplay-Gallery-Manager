import "@testing-library/jest-dom/vitest";

import type { MockedResponse } from "@apollo/client/testing";
import { MockedProvider } from "@apollo/client/testing/react";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { MANAGE_RUNTIME_SETTINGS } from "../api/manage";
import { bytesToGiB, formatStorageBytes, gibToBytes, ManageSettingsPage } from "./ManageSettingsPage";

afterEach(cleanup);

const runtimeSettings = {
  settingsRevision: 1, homeScope: "LIST", galleryCardScrubberEnabled: true, galleryDetailMediaFilterEnabled: true,
  galleryCardControlsVisible: true, mediaCardControlsVisible: true, detailPersonalControlsVisible: true,
  relatedLimit: 12, tagParentWeight: 0.5, tagMinimumScore: 0.2, tagMaximumDepth: 3,
  randomLimit: 24, randomStaticQuota: 0.7, randomGIFQuota: 0.15, randomVideoQuota: 0.15, randomGalleryRepeatDecay: 0.5,
  enhancedCacheMaximumBytes: 53687091200, minimumFreeBytes: 10737418240, minimumFreePercent: 0.05,
  automaticScanEnabled: false, automaticSchedulesSuspended: false, dailyBackupEnabled: true, dailyBackupRetention: 7,
  archiveMaxEntries: 10000, archiveMaxEntryBytes: 2147483648, archiveMaxTotalBytes: 2147483648,
  archiveMaxCompressionRatio: 200, archiveMaxImagePixels: 250000000,
};

describe("ManageSettingsPage cache status", () => {
  it("shows the cache path and occupied space as read-only deployment information", async () => {
    const mocks: MockedResponse[] = [{
      request: { query: MANAGE_RUNTIME_SETTINGS },
      result: { data: {
        manageRuntimeSettings: runtimeSettings,
        manageCacheStorage: { path: "/var/cache/cgm", byteSize: 156263337, fileCount: 216, baseByteSize: 3733456, enhancedByteSize: 152529881 },
      } },
    }];
    render(<MockedProvider mocks={mocks}><ManageSettingsPage /></MockedProvider>);

    expect(await screen.findByText("/var/cache/cgm")).toBeInTheDocument();
    expect(screen.getByText("149 MiB")).toBeInTheDocument();
    expect(screen.getByText("156,263,337 bytes · 216 files")).toBeInTheDocument();
		expect(screen.getByText("3.56 MiB")).toBeInTheDocument();
		expect(screen.getByText("145 MiB")).toBeInTheDocument();
		expect(screen.getByLabelText("Reclaimable cache limit (GiB)")).toHaveValue(50);
    expect(screen.queryByDisplayValue("/var/cache/cgm")).not.toBeInTheDocument();
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
