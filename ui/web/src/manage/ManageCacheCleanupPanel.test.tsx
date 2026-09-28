import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { afterEach, describe, expect, it, vi } from "vitest";
import { messages } from "../i18n/messages";
import { ManageCacheCleanupPanel, type CacheCleanupReview } from "./ManageCacheCleanupPanel";

const review: CacheCleanupReview = {
  referenced_bytes: 1024, obsolete_bytes: 512, pending_bytes: 0, pending_files: 0, failed_files: 0,
  orphan_bytes: 0, reclaimable_bytes: 512, partial: true, ignored_files: 1, grace_hours: 24,
  candidates: [{ id: "a".repeat(64), reason: "OBSOLETE", byte_size: 512 }],
};
const show = (locale: "en-GB" | "zh-CN" = "en-GB") => render(<IntlProvider locale={locale} messages={messages[locale]}><ManageCacheCleanupPanel /></IntlProvider>);
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

describe("cache lifecycle review", () => {
  it("does no scan on mount, requires preview, selection, password and CLEAN", async () => {
    const fetch = vi.fn().mockResolvedValueOnce({ ok: true, json: async () => review }).mockResolvedValueOnce({ ok: true, json: async () => ({ removed: 1, freed_bytes: 512, failed: 0, skipped: 0 }) });
    vi.stubGlobal("fetch", fetch); show();
    expect(fetch).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Preview cleanup" }));
    await screen.findByText("Superseded derivative");
    expect(screen.getByText(/bounded, partial preview/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Clean 0 selected/ })).toBeDisabled();
    fireEvent.click(screen.getByRole("checkbox"));
    fireEvent.click(screen.getByRole("button", { name: /Clean 1 selected/ }));
    expect(screen.getByRole("button", { name: "Execute cleanup" })).toBeDisabled();
    fireEvent.change(screen.getByLabelText("Owner password"), { target: { value: "password" } });
    fireEvent.change(screen.getByLabelText("Type CLEAN"), { target: { value: "CLEAN" } });
    fireEvent.click(screen.getByRole("button", { name: "Execute cleanup" }));
    await screen.findByText(/Removed 1 files/);
    expect(fetch.mock.calls[1][1].body).toBe(JSON.stringify({ ids: ["a".repeat(64)], password: "password", confirmation: "CLEAN" }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });
  it("discards stale selections without automatic retry and supports Chinese", async () => {
    const fetch = vi.fn().mockResolvedValueOnce({ ok: true, json: async () => review }).mockResolvedValueOnce({ ok: false, status: 409 });
    vi.stubGlobal("fetch", fetch); show("zh-CN");
    fireEvent.click(screen.getByRole("button", { name: "预览缓存清理" }));
    await screen.findByText("已被新版替代的派生图");
    fireEvent.click(screen.getByRole("checkbox"));
    fireEvent.click(screen.getByRole("button", { name: /清理所选 1 项/ }));
    fireEvent.change(screen.getByLabelText("所有者密码"), { target: { value: "password" } });
    fireEvent.change(screen.getByLabelText("输入 CLEAN"), { target: { value: "CLEAN" } });
    fireEvent.click(screen.getByRole("button", { name: "执行清理" }));
    await screen.findByText(/预览已变化/);
    await waitFor(() => expect(fetch).toHaveBeenCalledTimes(2));
    expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();
  });
});
