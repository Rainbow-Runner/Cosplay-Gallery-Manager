import { defineConfig, devices } from "@playwright/test";

const externalServer = process.env.CGM_E2E_EXTERNAL_SERVER === "1";
const browser = process.env.CGM_E2E_BROWSER || "chromium";
const browserDevice = browser === "firefox" ? devices["Desktop Firefox"]
  : browser === "webkit" ? devices["Desktop Safari"] : devices["Desktop Chrome"];

export default defineConfig({
  testDir: "./e2e",
  outputDir: "./e2e/test-results",
  fullyParallel: false,
  workers: 1,
  timeout: 120_000,
  expect: {
    timeout: 15_000,
    toHaveScreenshot: { animations: "disabled", maxDiffPixelRatio: 0.002 },
  },
  reporter: [["list"], ["html", { open: "never", outputFolder: "playwright-report" }]],
  use: {
    ...browserDevice,
    baseURL: "http://127.0.0.1:3210",
    channel: browser === "chromium" ? "chrome" : undefined,
    locale: "en-GB",
    timezoneId: "UTC",
    colorScheme: "dark",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  webServer: externalServer ? undefined : {
    command: "bash e2e/start-server.sh",
    url: "http://127.0.0.1:3210/healthz",
    reuseExistingServer: false,
    timeout: 120_000,
  },
});
