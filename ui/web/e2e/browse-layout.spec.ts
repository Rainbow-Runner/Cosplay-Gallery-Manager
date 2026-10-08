import { expect, test } from "@playwright/test";
import { readFileSync } from "node:fs";
import path from "node:path";

const styles = ["design-system.css", "index.css"]
  .map((name) => readFileSync(path.resolve("src/styles", name), "utf8"))
  .join("\n");
const longName = "A deliberately long character or coser name ".repeat(8);

test("metadata visibility save keeps readable primary colours including hover", async ({ page }) => {
  await page.setContent(`<style>${styles}</style><div class="manage-shell"><form class="settings-form"><button class="settings-metadata-visibility__save" type="button">Save metadata visibility</button></form></div>`);
  const button = page.getByRole("button", { name: "Save metadata visibility" });
  for (const hover of [false, true]) {
    if (hover) await button.hover();
    expect(await button.evaluate((element) => {
      const style = getComputedStyle(element);
      return { color: style.color, background: style.backgroundColor };
    })).toEqual({ color: "rgb(250, 250, 250)", background: "rgb(23, 23, 23)" });
  }
});

test("monthly timeline leaves only a compact rail beside the cards", async ({ page }) => {
  await page.setContent(`<style>${styles}</style><main class="browse-main"><div class="coser-month-timeline"><ol class="coser-month-timeline__months">
    ${["2026-10", "2026-09"].map((month) => `<li class="coser-month-timeline__month"><h2 class="coser-month-timeline__date"><time>${month}</time></h2><section class="gallery-grid">${Array.from({ length: 5 }, () => `<article class="gallery-card"><div class="cgm-card-media"></div><h2>${longName}</h2></article>`).join("")}</section></li>`).join("")}
    </ol></div></main>`);
  for (const width of [390, 768, 1280, 1920, 2560, 3840]) {
    await page.setViewportSize({ width, height: 900 });
    const metrics = await page.evaluate(() => {
      const month = document.querySelector(".coser-month-timeline__month")!.getBoundingClientRect();
      const grid = document.querySelector(".gallery-grid")!.getBoundingClientRect();
      return { rail: grid.left - month.left, ratio: grid.width / month.width, overflow: document.documentElement.scrollWidth > window.innerWidth };
    });
    expect(metrics.rail).toBeCloseTo(width < 768 ? 24 : 88, 0);
    if (width >= 1280) expect(metrics.ratio).toBeGreaterThan(0.9);
    expect(metrics.overflow).toBe(false);
  }
});

// Real production CSS in an isolated layout fixture: no API, owner or media data.
test("browse cards and banners fill wide viewports without scaling typography", async ({ page }) => {
  await page.setContent(`<style>${styles}</style><div class="browse-shell">
    <aside class="browse-sidebar"></aside><div class="browse-workspace">
      <main class="browse-main coser-detail"><section class="coser-profile">
        <div class="coser-banner"></div></section>
        <div class="gallery-grid">${Array.from({ length: 5 }, () => `<article class="gallery-card">
          <div class="cgm-card-media"></div><div class="gallery-card__body">
            <h2>${longName}</h2><p class="gallery-card__work">${longName}</p>
            <div class="gallery-card__people"><span class="gallery-card__cosers">
              <a class="gallery-card__coser-link"><span>${longName}</span></a>
            </span></div></div></article>`).join("")}</div>
        <p class="gallery-description">Readable prose retains a separate width limit.</p>
      </main></div></div>`);
  let previousCardWidth = 0;
  for (const width of [390, 768, 1280, 1536, 1920, 2560, 3840]) {
    await page.setViewportSize({ width, height: 900 });
    const metrics = await page.evaluate(() => {
      const main = document.querySelector<HTMLElement>(".browse-main")!;
      const workspace = document.querySelector<HTMLElement>(".browse-workspace")!;
      const grid = document.querySelector<HTMLElement>(".gallery-grid")!;
      const banner = document.querySelector<HTMLElement>(".coser-banner")!;
      const card = document.querySelector<HTMLElement>(".gallery-card")!;
      const font = (selector: string) => getComputedStyle(document.querySelector(selector)!).fontSize;
      return {
        mainWidth: main.getBoundingClientRect().width,
        workspaceWidth: workspace.getBoundingClientRect().width,
        contentWidth: main.clientWidth - parseFloat(getComputedStyle(main).paddingLeft) - parseFloat(getComputedStyle(main).paddingRight),
        gridWidth: grid.getBoundingClientRect().width,
        bannerWidth: banner.getBoundingClientRect().width,
        bannerHeight: banner.getBoundingClientRect().height,
        cardWidth: card.getBoundingClientRect().width,
        columns: getComputedStyle(grid).gridTemplateColumns.split(" ").length,
        fonts: [font(".gallery-card h2"), font(".gallery-card__work"), font(".gallery-card__coser-link")],
        colors: [".gallery-card__coser-link", ".gallery-card__coser-link > span", ".gallery-card__work", ".gallery-card__people"].map((selector) => getComputedStyle(document.querySelector(selector)!).color),
        titleWrap: getComputedStyle(document.querySelector(".gallery-card h2")!).whiteSpace,
        titleOverflow: getComputedStyle(document.querySelector(".gallery-card h2")!).textOverflow,
        proseWidth: document.querySelector(".gallery-description")!.getBoundingClientRect().width,
        overflow: document.documentElement.scrollWidth > window.innerWidth,
      };
    });
    expect(metrics.mainWidth).toBeCloseTo(metrics.workspaceWidth, 0);
    expect(metrics.gridWidth).toBeCloseTo(metrics.contentWidth, 0);
    // The profile contributes a 1px border on each side.
    expect(metrics.bannerWidth).toBeCloseTo(metrics.contentWidth - 2, 0);
    expect(metrics.bannerWidth / metrics.bannerHeight).toBeCloseTo(4, 1);
    expect(metrics.columns).toBe(width >= 1536 ? 5 : width >= 1024 ? 4 : width >= 768 ? 3 : 2);
    expect(metrics.fonts).toEqual(["16px", "12px", "14px"]);
    expect(metrics.colors).toEqual(["rgb(10, 10, 10)", "rgb(10, 10, 10)", "rgb(115, 115, 115)", "rgb(115, 115, 115)"]);
    expect(metrics.titleWrap).toBe("nowrap");
    expect(metrics.titleOverflow).toBe("ellipsis");
    expect(metrics.overflow).toBe(false);
    if (width >= 1920) {
      expect(metrics.cardWidth).toBeGreaterThan(previousCardWidth);
      expect(metrics.proseWidth).toBeLessThan(metrics.gridWidth);
    }
    previousCardWidth = metrics.cardWidth;
  }
});

test("management forms and explanatory text retain reading width limits", async ({ page }) => {
  await page.setViewportSize({ width: 2560, height: 900 });
  await page.setContent(`<style>${styles}</style><div class="manage-shell"><aside></aside><main>
    <form class="settings-form"><fieldset>Settings</fieldset></form>
    <form class="metadata-form">Metadata</form><section class="manage-panel">Panel</section>
    <p class="manage-help-intro">Explanatory text</p>
  </main></div>`);
  for (const [selector, cap] of [[".settings-form", 1280], [".metadata-form", 1040], [".manage-panel", 1040], [".manage-help-intro", 928]] as const) {
    const width = await page.locator(selector).evaluate((element) => element.getBoundingClientRect().width);
    expect(width).toBeLessThanOrEqual(cap);
    expect(width).toBeGreaterThan(cap - 1);
  }
});
