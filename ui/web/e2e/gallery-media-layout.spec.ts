import { expect, test } from "@playwright/test";

// Browser-only fixture: never reads/writes the user's business database/media.
test("gallery grid/justified switching, stable media identity, scroll anchor and mobile reflow", async ({ page }, testInfo) => {
  const pageErrors: string[] = [];
  page.on("pageerror", (error) => pageErrors.push(error.message));
  await page.setViewportSize({ width: 1440, height: 900 });
  const items = Array.from({ length: 60 }, (_, i) => {
    const width = i % 4 === 0 ? 480 : i % 4 === 1 ? 320 : i % 4 === 2 ? 480 : 160;
    const height = i % 4 === 0 ? 320 : i % 4 === 1 ? 480 : i % 4 === 2 ? 480 : 480;
    const resource = { itemUUID: `media-${i}`, contentRevision: 1, profileHash: "fixture", variant: "CARD_480", mimeType: "image/jpeg" };
    return { itemUUID: `media-${i}`, mediaKind: "STATIC_IMAGE", contentFormat: "IMAGE", imageCategory: "PHOTO", position: String(i), caption: `Media ${i}`, processingState: "READY", previewWidth: width, previewHeight: height, favorite: false, ratingHalfSteps: null, cardResource: resource, largeResource: { ...resource, variant: "LIGHTBOX_4096" } };
  });
  const card = { setID: "layout-fixture", slug: "layout-fixture", title: "Mixed landscape and portrait", collectionType: "COSPLAY", contentRating: "NON_ADULT", cover: { kind: "ITEM", revision: 1, managed: false, warning: false, resource: items[0].cardResource }, credits: [], creditCount: 0, characters: [], characterCount: 0, works: [], workCount: 0, shootDate: "", shootDatePrecision: "UNKNOWN", publishDate: "", publishDatePrecision: "UNKNOWN", addedAtUTC: "", mediaAddedStartUTC: "", mediaAddedEndUTC: "", mediaAddedStatus: "NONE", media: { photo: 60, selfie: 0, gif: 0, video: 0 }, favorite: false, ratingHalfSteps: null, scrubberCount: 0, scrubberRevision: 0 };
  const detail = { card: { __typename: "BrowseGalleryCard", ...card }, metadataRevision: 1, description: "", photographerName: "", studioName: "", availableBytes: 0, mediaParentDirectories: [], redirected: false, imageCaptureStart: "", imageCaptureEnd: "", videoCaptureStart: "", videoCaptureEnd: "", credits: [], tags: [], externalLinks: [] };
  await page.route("**/session/status", (route) => route.fulfill({ json: { setupComplete: true, authenticated: true, trustedMode: false } }));
  await page.route("**/maintenance/status", (route) => route.fulfill({ json: { state: "NORMAL" } }));
  await page.route("**/graphql", async (route) => {
    const { operationName } = route.request().postDataJSON();
    const results: Record<string, unknown> = {
      GalleryDetail: { galleryDetail: detail },
      GalleryMemberIndex: { galleryMemberIndex: { setID: card.setID, metadataRevision: 1, scanRevision: 1, items } },
      BrowseUISettings: { browseUISettings: { settingsRevision: 1, galleryScrubberEnabled: false, detailMediaFilterEnabled: false, galleryAnimatedPlaybackLimit: 12, galleryAnimatedLockIntervalMS: 800, cardFavoriteControlVisible: true, cardRatingSummaryVisible: false, detailRatingControlVisible: false } },
      RelatedGalleries: { relatedGalleries: [] },
      RecordGalleryView: { recordGalleryView: true },
    };
    await route.fulfill({ json: { data: results[operationName] ?? {} } });
  });
  await page.route("**/resource/**", async (route) => {
    const index = Number(route.request().url().match(/media-(\d+)/)?.[1] ?? 0);
    const { previewWidth: width, previewHeight: height } = items[index];
    await route.fulfill({ contentType: "image/svg+xml", body: `<svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}" viewBox="0 0 ${width} ${height}"><rect width="100%" height="100%" fill="hsl(${index * 29 % 360} 35% 75%)"/><rect x="2" y="2" width="${width - 4}" height="${height - 4}" fill="none" stroke="#404040" stroke-width="4"/><text x="50%" y="50%" text-anchor="middle" font-size="24">${width} × ${height}</text></svg>` });
  });
  await page.goto("/gallery/layout-fixture");
  const tiles = page.locator(".media-tile");
  try { await expect(tiles).toHaveCount(24); }
  catch (error) {
    console.log("Layout fixture page errors:", pageErrors, "Root:", await page.locator("#root").innerHTML());
    throw error;
  }
  await expect(page.getByRole("button", { name: "Card grid" })).toHaveAttribute("aria-pressed", "true");
  const layoutSwitch = page.getByRole("group", { name: "Media layout" });
  const favourite = page.getByRole("button", { name: "Favourite gallery", exact: true });
  const checkCompactHeader = async () => {
    await expect(page.locator(".gallery-media-toolbar")).toHaveCount(0);
    await expect(page.locator(".gallery-detail__actions .media-layout-switch")).toHaveCount(1);
    const layoutBox = (await layoutSwitch.boundingBox())!, favouriteBox = (await favourite.boundingBox())!;
    expect(layoutBox.x + layoutBox.width).toBeLessThanOrEqual(favouriteBox.x);
    expect(layoutBox.y).toBeCloseTo(favouriteBox.y);
    expect(layoutBox.height).toBeCloseTo(favouriteBox.height);
    expect(layoutBox.width).toBeLessThanOrEqual(72);
    await expect(page.getByRole("button", { name: "Justified rows" })).toHaveAttribute("title", "Justified rows");
  };
  await checkCompactHeader();
  await page.locator(".media-tile").evaluateAll((elements) => elements.forEach((element) => element.setAttribute("data-original-node", "yes")));
  await page.getByRole("button", { name: "Justified rows" }).click();
  await expect(page.locator(".media-sequence")).toHaveAttribute("data-layout", "JUSTIFIED");
  await expect(page.locator('.media-tile[data-original-node="yes"]')).toHaveCount(24);
  const geometry = await tiles.evaluateAll((elements) => elements.map((element) => {
    const rect = element.getBoundingClientRect(), image = element.querySelector("img")!;
    return { uuid: (element as HTMLElement).dataset.itemUuid, x: rect.x, y: rect.y, width: rect.width, height: rect.height, fit: getComputedStyle(image).objectFit };
  }));
  expect(geometry.map((entry) => entry.uuid)).toEqual(items.slice(0, 24).map((item) => item.itemUUID));
  for (let i = 0; i < geometry.length; i++) {
    expect(geometry[i].fit).toBe("contain");
    expect(geometry[i].width).toBeLessThanOrEqual(480.1);
    if (i > 0) {
      expect(geometry[i].y).toBeGreaterThanOrEqual(geometry[i - 1].y);
      if (geometry[i].y === geometry[i - 1].y) {
        expect(geometry[i].height).toBeCloseTo(geometry[i - 1].height);
        expect(geometry[i].x).toBeGreaterThanOrEqual(geometry[i - 1].x + geometry[i - 1].width);
      }
    }
  }
  await page.screenshot({ path: testInfo.outputPath("justified-desktop.png"), fullPage: true });
  await page.locator(".load-more").click();
  await expect(tiles).toHaveCount(48);
  await page.evaluate(() => window.scrollTo(0, 700));
  const anchor = await tiles.evaluateAll((elements) => elements.map((element) => ({ uuid: (element as HTMLElement).dataset.itemUuid!, top: element.getBoundingClientRect().top, bottom: element.getBoundingClientRect().bottom })).filter((entry) => entry.bottom > 0 && entry.top < innerHeight).sort((a, b) => Math.abs(a.top) - Math.abs(b.top))[0]);
  await page.getByRole("button", { name: "Card grid" }).evaluate((element: HTMLButtonElement) => element.click());
  await expect(page.locator(".media-sequence")).toHaveAttribute("data-layout", "GRID");
  await expect.poll(async () => Math.abs((await page.locator(`[data-item-uuid="${anchor.uuid}"]`).boundingBox())!.y - anchor.top)).toBeLessThan(3);
  await page.getByRole("button", { name: "Justified rows" }).evaluate((element: HTMLButtonElement) => element.click());
  await page.setViewportSize({ width: 390, height: 844 });
  await page.evaluate(() => window.scrollTo(0, 0));
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await checkCompactHeader();
  await page.screenshot({ path: testInfo.outputPath("justified-mobile-viewport.png") });
  await page.screenshot({ path: testInfo.outputPath("justified-mobile.png"), fullPage: true });
  await page.reload();
  await expect(page.getByRole("button", { name: "Justified rows" })).toHaveAttribute("aria-pressed", "true");
  await expect(tiles).toHaveCount(24);
  expect(pageErrors).toEqual([]);
});
