import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import path from "node:path";

const password = "offline e2e owner password";
const runtime = path.resolve("e2e/.runtime");
const libraryRoot = path.join(runtime, "library");

async function expectAccessible(page: Page) {
  const result = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"])
    .analyze();
  expect(result.violations, JSON.stringify(result.violations, null, 2)).toEqual([]);
}

async function login(page: Page, expectedURL: string | RegExp = "/") {
  await page.getByLabel("Password", { exact: true }).fill(password);
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page).toHaveURL(expectedURL);
}

async function waitForReadyItems(page: Page) {
  await expect.poll(async () => {
    await page.waitForTimeout(500);
    await page.reload();
    await page.getByText("3 members").waitFor();
    return page.getByText("READY", { exact: true }).count();
  }, { timeout: 30_000 }).toBeGreaterThan(0);
}

test("offline owner lifecycle, backup restore and accessibility matrix", async ({ page, browserName }) => {
  await page.goto("/setup");
  await expect(page.getByRole("heading", { name: "First-time setup" })).toBeVisible();
  await expectAccessible(page);
  await page.getByRole("button", { name: "Continue" }).click();

  await page.getByLabel("Password", { exact: true }).fill(password);
  await page.getByLabel("Confirm password").fill(password);
  await page.getByRole("button", { name: "Continue" }).click();
  await page.getByLabel("Language").selectOption("en-GB");
  await page.getByLabel("Capture timezone").fill("UTC");
  await page.getByRole("button", { name: "Continue" }).click();
  await page.getByLabel("Coser metadata root").fill(path.join(runtime, "cosers"));
  await page.getByLabel("Backup root").fill(path.join(runtime, "backups"));
  await page.getByRole("button", { name: "Continue" }).click();
  await expect(page.getByText("The first media scan will not start automatically.")).toBeVisible();
  const createLocalLibrary = page.getByRole("button", {
    name: "Create local library",
  });
  await expect.poll(async () =>
    new URL(page.url()).pathname === "/login" || await createLocalLibrary.isEnabled(),
  ).toBe(true);
  if (new URL(page.url()).pathname !== "/login") {
    await createLocalLibrary.click({ noWaitAfter: true });
  }
  await expect(page).toHaveURL("/login");
  await login(page);

  await page.goto("/manage/cosers");
  await page.getByLabel("Name", { exact: true }).fill("Offline E2E Coser");
  await page.getByLabel("Profile summary").fill("Local-only lifecycle fixture.");
  await page.getByRole("button", { name: "Create", exact: true }).click();
  await expect(page.getByText("Saved", { exact: true })).toBeVisible();

  await page.goto("/manage/libraries");
  const addLibrary = page.locator("details").filter({
    has: page.getByText("Add media library", { exact: true }),
  });
  await addLibrary.getByText("Add media library", { exact: true }).click();
  await addLibrary.getByLabel("Name", { exact: true }).fill("Offline fixture");
  await addLibrary.getByLabel("Absolute path").fill(libraryRoot);
  await addLibrary.getByLabel("Read-only source").uncheck();
  await addLibrary.getByRole("button", { name: "Create", exact: true }).click();
  await expect(page.getByRole("button", { name: /Offline fixture/ })).toBeVisible();
  const addRule = page.locator("details").filter({
    has: page.getByText("Add deterministic rule", { exact: true }),
  });
  await addRule.getByText("Add deterministic rule", { exact: true }).click();
  await addRule.getByLabel("Name", { exact: true }).fill("Direct gallery directory");
  await addRule.getByLabel("Enable rule").check();
  await addRule.getByRole("button", { name: "Add rule" }).click();
  await expect(page.getByText(/FIXED_DEPTH · ON/)).toBeVisible();
  await page.getByRole("button", { name: "Scan selected library" }).click();
  const candidate = page.locator(".candidate-card").filter({ hasText: "playwright-gallery" });
  await expect(candidate).toContainText("3 media");
  await candidate.getByRole("button", { name: "Create DRAFT" }).click();
  await expect(page).toHaveURL(/\/manage\/gallery\/[0-9a-f-]+$/);
  const galleryURL = page.url();

  await page.getByLabel("Title", { exact: true }).fill("Offline E2E Gallery");
  await page.getByLabel("Rating").selectOption("NON_ADULT");
  await page.getByRole("button", { name: "Save metadata" }).click();
  await expect(page.getByText("Saved", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "source", exact: true }).click();
  await page.getByRole("button", { name: "Scan source now" }).click();
  await expect(page.getByText("Source scan completed")).toBeVisible();
  await page.getByRole("button", { name: "media", exact: true }).click();
  await waitForReadyItems(page);
  await expect(page.getByText("3 members")).toBeVisible();
  await page.getByRole("button", { name: "Cover", exact: true }).first().click();
  await expect(page.getByText("Cover updated")).toBeVisible();

  await page.getByRole("button", { name: "cast", exact: true }).click();
  await page.getByRole("button", { name: "Add Coser" }).click();
  await page.getByLabel("Search Coser").fill("Offline E2E Coser");
  await page.getByRole("option", { name: /Offline E2E Coser/ }).click();
  await page.getByRole("button", { name: "Save all relations" }).click();
  await expect(page.getByText("People, characters and tags saved")).toBeVisible();
  await page.getByRole("button", { name: "Activate" }).click();
  await expect(page.getByText(/ACTIVE · revision/)).toBeVisible();
  await page.getByRole("button", { name: "manifest", exact: true }).click();
  await page.getByRole("button", { name: "Push database → Manifest" }).click();
  await expect(page.getByText("Manifest Push completed")).toBeVisible();

  await page.goto("/");
  const galleryLink = page.getByRole("link", { name: "Offline E2E Gallery" }).last();
  await expect(galleryLink).toBeVisible();
  await expectAccessible(page);
  if (browserName === "chromium") {
    await expect(page).toHaveScreenshot("browse-desktop.png", { fullPage: true });
  }
  await galleryLink.click();
  await expect(page.getByRole("heading", { name: "Offline E2E Gallery" })).toBeVisible();
  await expect(page.getByText("3", { exact: true }).first()).toBeVisible();

  await page.setViewportSize({ width: 390, height: 844 });
  await expectAccessible(page);
  if (browserName === "chromium") {
    await expect(page).toHaveScreenshot("gallery-mobile.png", { fullPage: true });
  }
  await page.keyboard.press("Tab");
  await expect(page.locator(":focus")).toBeVisible();
  await page.setViewportSize({ width: 1280, height: 720 });

  await page.goto("/manage/operations");
  await expectAccessible(page);
  await page.getByRole("button", { name: "Create full backup" }).click();
  await expect(page.getByText(/Full backup ready:/)).toBeVisible({ timeout: 30_000 });
  const manualBackup = page.locator(".backup-table tbody tr").filter({ hasText: "MANUAL_FULL" }).first();
  await manualBackup.getByRole("button", { name: "Restore…" }).click();
  await page.getByLabel(/Type RESTORE/).fill("RESTORE");
  await page.getByRole("button", { name: "Create safety backup and restore" }).click();
  await expect(page).toHaveURL(/\/login\?restored=1$/, { timeout: 60_000 });
  await login(page, /\/maintenance$/);

  await expect(page).toHaveURL("/maintenance");
  await expect(page.getByRole("heading", { name: "Map restored media libraries" })).toBeVisible();
  await page.getByLabel("Absolute directory on this machine").fill(libraryRoot);
  await page.getByLabel("Owner password").fill(password);
  await page.getByLabel(/Type MAP/).fill("MAP");
  await page.getByRole("button", { name: "Confirm path mappings" }).click();
  await expect(page.getByText("Media library decisions saved. No media scan was started.")).toBeVisible();
  await page.getByRole("button", { name: "Validate environment and resume tasks" }).click();
  await expect(page).toHaveURL("/manage/operations");

  await page.goto(`${galleryURL}?tab=source`);
  await page.getByRole("button", { name: "Scan source now" }).click();
  await expect(page.getByText("Source scan completed")).toBeVisible();
  await page.goto("/");
  await expect(page.getByRole("link", { name: "Offline E2E Gallery" }).first()).toBeVisible();
});
