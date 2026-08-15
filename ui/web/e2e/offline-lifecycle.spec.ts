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

  await page.goto("/manage/settings");
  await expect(page.getByRole("heading", { name: "Settings" })).toBeVisible();
  const settingsControlColours = await page.locator('.settings-form input[type="number"], .settings-form select').evaluateAll((elements) =>
    Array.from(new Set(elements.flatMap((element) => {
      const style = getComputedStyle(element);
      return [`${style.backgroundColor}|${style.color}`];
    }))),
  );
  expect(settingsControlColours).toEqual(["rgb(255, 255, 255)|rgb(10, 10, 10)"]);
  await expectAccessible(page);

  await page.goto("/manage/cosers");
  await page.getByLabel("Name (required)", { exact: true }).fill("Offline E2E Coser");
  await page.getByLabel("Profile summary").fill("Local-only lifecycle fixture.");
  await page.getByRole("button", { name: "Create", exact: true }).click();
  await expect(page.getByText("Saved", { exact: true })).toBeVisible();

  await page.goto("/manage/libraries");
  const addLibrary = page.locator("details").filter({
    has: page.getByText("Add media library", { exact: true }),
  });
  await addLibrary.getByText("Add media library", { exact: true }).click();
  await addLibrary.getByLabel("Name (required)", { exact: true }).fill("Offline fixture");
  await addLibrary.getByLabel("Absolute path (required)").fill(libraryRoot);
  await addLibrary.getByLabel("Read-only source").uncheck();
  await addLibrary.getByRole("button", { name: "Create", exact: true }).click();
  await expect(page.getByRole("button", { name: /Offline fixture/ })).toBeVisible();
  const addRule = page.locator("details").filter({
    has: page.getByText("Add deterministic rule", { exact: true }),
  });
  await addRule.getByText("Add deterministic rule", { exact: true }).click();
  await addRule.getByLabel("Name (required)", { exact: true }).fill("Direct gallery directory");
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
  await page.getByRole("checkbox", { name: /Auto-exclude newly discovered media in Gallery root/ }).uncheck();
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
  await expect(page.getByRole("listbox")).toHaveCount(0);
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
  const galleryGrid = page.locator(".gallery-grid").filter({ has: galleryLink });
  const galleryCount = page.getByLabel("Media count 2P 1G");
  const galleryFavourite = page.getByRole("button", { name: "Favourite", exact: true });
  await expect(galleryCount).toBeVisible();
  expect(await galleryCount.evaluate((element) => {
    const style = getComputedStyle(element);
    return {
      backgroundColor: style.backgroundColor,
      backdropFilter: style.backdropFilter,
      borderRadius: style.borderRadius,
      fontFamily: style.fontFamily,
      fontSize: style.fontSize,
      fontWeight: style.fontWeight,
      letterSpacing: style.letterSpacing,
      lineHeight: style.lineHeight,
      padding: style.padding,
    };
  })).toEqual({
    backgroundColor: "rgba(0, 0, 0, 0)",
    backdropFilter: "none",
    borderRadius: "0px",
    fontFamily: expect.stringContaining("Inter"),
    fontSize: "12px",
    fontWeight: "400",
    letterSpacing: "normal",
    lineHeight: "16px",
    padding: "0px",
  });
  await page.mouse.move(0, 0);
  await expect.poll(async () => galleryFavourite.evaluate((element) => getComputedStyle(element).opacity)).toBe("0");
  for (const [width, columns] of [[390, 2], [768, 3], [1024, 4], [1536, 5]] as const) {
    await page.setViewportSize({ width, height: 844 });
    await expect.poll(async () => galleryGrid.evaluate((element) => getComputedStyle(element).gridTemplateColumns.split(" ").length)).toBe(columns);
    expect(await galleryGrid.evaluate((element) => getComputedStyle(element).gap)).toBe("16px");
  }
  await page.setViewportSize({ width: 1280, height: 720 });
  await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur());
  await page.mouse.move(0, 0);
  await expect.poll(async () => galleryFavourite.evaluate((element) => getComputedStyle(element).opacity)).toBe("0");
  await expect(page.locator(".gallery-card__kind")).toHaveCount(0);
  const galleryPersonLink = page.locator(".gallery-card__coser-link").filter({ hasText: "Offline E2E Coser" });
  expect(await galleryPersonLink.evaluate((element) => {
    const style = getComputedStyle(element);
    const avatar = element.querySelector<HTMLElement>(".cgm-avatar");
    return {
      height: style.height,
      gap: style.gap,
      fontSize: style.fontSize,
      fontWeight: style.fontWeight,
      lineHeight: style.lineHeight,
      avatarWidth: avatar ? getComputedStyle(avatar).width : "",
      avatarHeight: avatar ? getComputedStyle(avatar).height : "",
    };
  })).toEqual({ height: "32px", gap: "6px", fontSize: "14px", fontWeight: "500", lineHeight: "14px", avatarWidth: "32px", avatarHeight: "32px" });
  await expectAccessible(page);
  if (browserName === "chromium") {
    await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur());
    await page.mouse.move(0, 0);
    await expect.poll(async () => galleryFavourite.evaluate((element) => getComputedStyle(element).opacity)).toBe("0");
    await expect(page).toHaveScreenshot("browse-desktop.png", { fullPage: true });
  }
  await page.locator(".gallery-card__poster-frame").hover();
  await expect.poll(async () => galleryFavourite.evaluate((element) => getComputedStyle(element).opacity)).toBe("1");
  await page.mouse.move(0, 0);
  await expect.poll(async () => galleryFavourite.evaluate((element) => getComputedStyle(element).opacity)).toBe("0");
  await galleryFavourite.focus();
  await expect.poll(async () => galleryFavourite.evaluate((element) => getComputedStyle(element).opacity)).toBe("1");
  await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur());

  await page.goto("/list");
  await expect(page.getByRole("link", { name: "Offline E2E Gallery" })).toHaveCount(0);
  await page.goto("/magic");
  await expect(page.getByRole("link", { name: "Offline E2E Gallery" })).toHaveCount(0);
  await page.goto("/albums");
  await expect(page.getByRole("link", { name: "Offline E2E Gallery" }).last()).toBeVisible();
  await page.goto("/cosers");
  await expect(page.getByRole("link", { name: "Offline E2E Coser" })).toHaveCount(0);
  await page.goto("/models");
  const modelIndexLink = page.getByRole("link", { name: "Offline E2E Coser" });
  await expect(modelIndexLink).toHaveAttribute("href", /^\/model\/.+/);
  const avatarBox = await modelIndexLink.locator(".entity-index__avatar").boundingBox();
  const nameBox = await modelIndexLink.locator("strong").boundingBox();
  const nameStyle = await modelIndexLink.locator("strong").evaluate((element) => {
    const style = getComputedStyle(element);
    return { fontSize: style.fontSize, fontWeight: style.fontWeight, lineHeight: style.lineHeight };
  });
  expect(avatarBox).not.toBeNull();
  expect(nameBox).not.toBeNull();
  expect(Math.abs((avatarBox!.y + avatarBox!.height / 2) - (nameBox!.y + nameBox!.height / 2))).toBeLessThan(1);
  expect(nameStyle).toEqual({ fontSize: "14px", fontWeight: "500", lineHeight: "14px" });
  await page.goto("/");
  const modelLink = page.getByRole("link", { name: "Offline E2E Coser" });
  await expect(modelLink).toHaveAttribute("href", /^\/model\/.+/);
  const modelPath = await modelLink.getAttribute("href");
  await modelLink.click();
  await expect(page).toHaveURL(/\/model\/.+/);
  await expect(page.getByRole("navigation", { name: "Breadcrumb" })).toContainText("Offline E2E Coser");
  await expectAccessible(page);
  if (browserName === "chromium") {
    await expect(page).toHaveScreenshot("model-detail-desktop.png", { fullPage: true });
    await page.setViewportSize({ width: 390, height: 844 });
    await expect(page).toHaveScreenshot("model-detail-mobile.png");
    await page.setViewportSize({ width: 1280, height: 720 });
  }

  await page.goto(modelPath!.replace("/model/", "/coser/"));
  await expect(page.getByRole("heading", { name: "Offline E2E Coser" })).toBeVisible();
  const socialAccounts = page.locator(".social-accounts");
  await expect(socialAccounts.locator(":scope > .social-account")).toHaveCount(6);
  await expect(socialAccounts.locator(":scope > a")).toHaveCount(0);
  expect(await socialAccounts.evaluate((element) => {
    const children = Array.from(element.children) as HTMLElement[];
    return {
      height: getComputedStyle(element).height,
      gap: getComputedStyle(element).gap,
      titles: children.map((child) => child.title),
      colors: children.map((child) => getComputedStyle(child).color),
      sizes: children.map((child) => {
        const svg = child.querySelector<SVGElement>("svg")!;
        const style = getComputedStyle(svg);
        return `${style.width}x${style.height}`;
      }),
    };
  })).toEqual({
    height: "24px",
    gap: "4px",
    titles: ["X", "Facebook", "Instagram", "微博", "Patreon", "Linktree"],
    colors: Array(6).fill("rgb(153, 161, 175)"),
    sizes: ["20pxx20px", "24pxx24px", "24pxx24px", "24pxx24px", "24pxx24px", "24pxx24px"],
  });
  await expectAccessible(page);
  if (browserName === "chromium") {
    await expect(page).toHaveScreenshot("coser-detail-desktop.png", { fullPage: true });
    await page.setViewportSize({ width: 390, height: 844 });
    await page.evaluate(() => {
      window.scrollTo(0, 0);
      for (const element of document.querySelectorAll<HTMLElement>("*")) element.scrollTop = 0;
    });
    await expect(page.locator(".browse-topbar")).toBeInViewport();
    await expect(page).toHaveScreenshot("coser-detail-mobile.png");
    await page.setViewportSize({ width: 1280, height: 720 });
  }
  const manageCoserLink = page.getByRole("link", { name: "Manage Offline E2E Coser" });
  await expect(manageCoserLink).toHaveAttribute("href", /^\/manage\/cosers\?uuid=.+/);
  await manageCoserLink.click();
  await expect(page).toHaveURL(/\/manage\/cosers\?uuid=.+/);
  await expect(page.getByRole("heading", { name: "Coser" })).toBeVisible();
  await expect(page.getByLabel("Name (required)", { exact: true })).toHaveValue("Offline E2E Coser");
  await page.goto("/");
  await galleryLink.click();
  await expect(page.getByRole("heading", { name: "Offline E2E Gallery" })).toBeVisible();
  await expect(page.getByText("2P 1G", { exact: true })).toBeVisible();
  const galleryHeading = page.locator(".gallery-detail__title h1");
  if (browserName === "chromium") {
    await page.setViewportSize({ width: 1440, height: 1000 });
    const originalHeading = await galleryHeading.textContent();
    await galleryHeading.evaluate((element) => { element.textContent = "Offline E2E Gallery — Complete Desktop Presentation Title 2026"; });
    const desktopHeadingMetrics = await galleryHeading.evaluate((element) => {
      const style = getComputedStyle(element);
      return {
        contentHeight: element.getBoundingClientRect().height - Number.parseFloat(style.paddingTop) - Number.parseFloat(style.paddingBottom),
        lineHeight: Number.parseFloat(style.lineHeight),
        maxWidth: style.maxWidth,
      };
    });
    expect(desktopHeadingMetrics.maxWidth).toBe("none");
    expect(desktopHeadingMetrics.contentHeight).toBeLessThanOrEqual(desktopHeadingMetrics.lineHeight * 1.1);
    await galleryHeading.evaluate((element, title) => { element.textContent = title; }, originalHeading);
    await page.mouse.move(1400, 24);
    await expect(page).toHaveScreenshot("gallery-detail-desktop.png", { fullPage: true });
  }

  const moreDetails = page.locator("details.gallery-detail__more");
  await moreDetails.locator("summary").click();
  await expect(moreDetails).toHaveAttribute("open", "");
  await expect(moreDetails.getByText(path.join(libraryRoot, "playwright-gallery"), { exact: true })).toBeVisible();
  await expect(moreDetails.getByRole("link", { name: "Manage this gallery" })).toHaveAttribute("href", /\/manage\/gallery\/[0-9a-f-]+\?tab=media$/);
  await page.getByRole("heading", { name: "Offline E2E Gallery" }).click();
  await expect(moreDetails).not.toHaveAttribute("open", "");

  await page.setViewportSize({ width: 1440, height: 320 });
  await page.evaluate(() => window.scrollTo(0, 0));
  const desktopSidebarY = (await page.locator(".browse-sidebar").boundingBox())!.y;
  await page.evaluate(() => window.scrollTo(0, 240));
  await expect.poll(async () => (await page.locator(".browse-topbar").boundingBox())!.y).toBeLessThan(-100);
  expect((await page.locator(".browse-sidebar").boundingBox())!.y).toBe(desktopSidebarY);
  await page.evaluate(() => window.scrollTo(0, 0));

  await page.setViewportSize({ width: 390, height: 844 });
  expect(await page.locator(".browse-topbar").evaluate((element) => getComputedStyle(element).position)).toBe("sticky");
  const originalMobileHeading = await galleryHeading.textContent();
  await galleryHeading.evaluate((element) => { element.textContent = "W".repeat(300); });
  const mobileTitleBox = await page.locator(".gallery-detail__title").boundingBox();
  const mobileActionsBox = await page.locator(".gallery-detail__actions").boundingBox();
  expect(mobileTitleBox).not.toBeNull();
  expect(mobileActionsBox).not.toBeNull();
  expect(mobileActionsBox!.y).toBeGreaterThanOrEqual(mobileTitleBox!.y + mobileTitleBox!.height);
  expect(await galleryHeading.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await galleryHeading.evaluate((element, title) => { element.textContent = title; }, originalMobileHeading);
  const mobileMenu = page.getByRole("button", { name: "Open navigation" });
  await mobileMenu.click();
  await expect(page.getByRole("dialog", { name: "Primary navigation" })).toBeVisible();
  await expectAccessible(page);
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog", { name: "Primary navigation" })).toBeHidden();
  await expect(mobileMenu).toBeFocused();
  await expectAccessible(page);
  if (browserName === "chromium") {
    await expect(page).toHaveScreenshot("gallery-mobile.png", { fullPage: true });
  }
  await page.keyboard.press("Tab");
  await expect(page.locator(":focus")).toBeVisible();
  await page.setViewportSize({ width: 1280, height: 720 });

  const mediaTiles = page.locator(".media-tile");
  await expect(mediaTiles).toHaveCount(3);
  const firstFavourite = mediaTiles.nth(0).locator(".media-tile__favorite");
  await firstFavourite.click();
  await expect(firstFavourite).toHaveAttribute("aria-pressed", "true");
  const secondMediaMenu = mediaTiles.nth(1).locator("details.media-tile__menu");
  await expect(secondMediaMenu.locator("summary")).not.toHaveAttribute("title", "Media actions");
  await secondMediaMenu.locator("summary").click();
  await expect(secondMediaMenu).toHaveAttribute("open", "");
  await page.getByRole("heading", { name: "Offline E2E Gallery" }).click();
  await expect(secondMediaMenu).not.toHaveAttribute("open", "");
  await secondMediaMenu.locator("summary").click();
  await mediaTiles.nth(1).getByRole("button", { name: "Set as cover" }).click();
  await expect(page.getByText("Cover updated")).toBeVisible();
  await expect(mediaTiles.nth(1).locator(".media-tile__cover")).toBeVisible();
  await mediaTiles.nth(1).locator(".media-tile__open").click();
  await expect(page).toHaveURL(/\/gallery\/[^?]+\?item=/);
  const mediaDialog = page.getByRole("dialog");
  await expect(mediaDialog).toBeVisible();
  await expect(mediaDialog.getByRole("button", { name: "Favourite media" })).toHaveAttribute("title", "Favourite media");
  await expect(mediaDialog.getByRole("combobox", { name: "Media rating" })).toHaveAttribute("title", "Media rating");
  await expect(mediaDialog.getByRole("button", { name: "Current cover" })).toHaveAttribute("title", "Current cover");
  await expect(mediaDialog.getByRole("link", { name: "Open media details" })).toHaveAttribute("title", "Open media details");
  await expect(mediaDialog.getByRole("button", { name: "Close media viewer" })).toHaveAttribute("title", "Close media viewer");
  await page.getByRole("button", { name: "Close media viewer" }).click();
  await expect(page).not.toHaveURL(/\?item=/);
  await expect(mediaTiles.nth(1)).toBeInViewport();

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
