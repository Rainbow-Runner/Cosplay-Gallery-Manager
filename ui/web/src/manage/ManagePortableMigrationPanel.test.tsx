import "@testing-library/jest-dom/vitest";

import { MockedProvider } from "@apollo/client/testing/react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { IntlProvider } from "react-intl";

import { MANAGE_LIBRARIES, MANAGE_PORTABLE_MIGRATION } from "../api/manage";
import { ManagePortableMigrationPanel } from "./ManagePortableMigrationPanel";

afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

describe("ManagePortableMigrationPanel", () => {
  it("states the non-portable boundary and requires reauthentication plus exact confirmation", async () => {
    render(<MockedProvider mocks={[
      { request: { query: MANAGE_PORTABLE_MIGRATION, variables: { importID: null, mergeID: null } }, result: { data: { managePortableMigration: { imports: [], merges: [], conflicts: [], mappings: [], rebuilds: [], owner: null }, manageMaintenance: { state: "NORMAL", restoreBackupID: null, lastErrorCode: "", updatedAt: "2026-09-10T00:00:00Z" } } } },
      { request: { query: MANAGE_LIBRARIES }, result: { data: { manageLibraries: [] } } },
    ]}><IntlProvider locale="zh-CN" messages={{}}><ManagePortableMigrationPanel /></IntlProvider></MockedProvider>);

    expect(screen.getByText(/不会迁移：Gallery 地址、评分、最后浏览时间/)).toBeInTheDocument();
    const exportButton = screen.getByRole("button", { name: "导出包…" });
    expect(exportButton).toBeDisabled();
    fireEvent.change(screen.getByLabelText("导出目标路径（服务器/容器内）"), { target: { value: "/srv/transfer/catalog.cgm-portable.zip" } });
    fireEvent.click(exportButton);

    const confirmButton = screen.getByRole("button", { name: "确认执行" });
    expect(confirmButton).toBeDisabled();
    fireEvent.change(screen.getByLabelText("所有者密码"), { target: { value: "owner secret" } });
    fireEvent.change(screen.getByLabelText("输入 EXPORT 继续"), { target: { value: "EXPORT" } });
    expect(confirmButton).toBeEnabled();
  });

  it("offers the matching recovery action for interrupted portable maintenance", async () => {
    const mergeID = "11111111-1111-4111-8111-111111111111";
    render(<MockedProvider mocks={[
      { request: { query: MANAGE_PORTABLE_MIGRATION, variables: { importID: null, mergeID: null } }, result: { data: { managePortableMigration: { imports: [], merges: [], conflicts: [], mappings: [], rebuilds: [], owner: null }, manageMaintenance: { state: "PORTABLE_MERGING", restoreBackupID: mergeID, lastErrorCode: "", updatedAt: "2026-09-10T00:00:00Z" } } } },
      { request: { query: MANAGE_LIBRARIES }, result: { data: { manageLibraries: [] } } },
    ]}><IntlProvider locale="zh-CN" messages={{}}><ManagePortableMigrationPanel /></IntlProvider></MockedProvider>);

    expect(await screen.findByText("检测到中断的迁移维护状态")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "执行安全恢复…" }));
    expect(screen.getByRole("heading", { name: "RECOVER_MERGE" })).toBeInTheDocument();
    expect(screen.getByLabelText("输入 RECOVER 继续")).toBeInTheDocument();
  });

  it("selects an import package independently and requires a valid full check", async () => {
    const packageEntry = { name: "catalog.zip", path: "/transfer/catalog.zip", size: 2048, createdAt: "2026-09-22T00:00:00Z", formatVersion: 2, checkStatus: "UNCHECKED" };
    const fetchMock = vi.fn().mockResolvedValueOnce({ ok: true, json: async () => ({ root: "/transfer", packages: [packageEntry] }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ ...packageEntry, checkStatus: "VALID" }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ root: "/transfer", packages: [packageEntry] }) });
    vi.stubGlobal("fetch", fetchMock);
    render(<MockedProvider mocks={[
      { request: { query: MANAGE_PORTABLE_MIGRATION, variables: { importID: null, mergeID: null } }, result: { data: { managePortableMigration: { imports: [], merges: [], conflicts: [], mappings: [], rebuilds: [], owner: null }, manageMaintenance: { state: "NORMAL", restoreBackupID: null, lastErrorCode: "", updatedAt: "2026-09-10T00:00:00Z" } } } },
      { request: { query: MANAGE_LIBRARIES }, result: { data: { manageLibraries: [] } } },
    ]}><IntlProvider locale="zh-CN" messages={{}}><ManagePortableMigrationPanel /></IntlProvider></MockedProvider>);
    await screen.findByRole("option", { name: "catalog.zip" });
    fireEvent.change(screen.getByLabelText("导入迁移包（/transfer）"), { target: { value: "catalog.zip" } });
    expect(screen.getByText("2.0 KiB")).toBeInTheDocument();
    const importButton = screen.getByRole("button", { name: "导入空业务库…" });
    expect(importButton).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "导入前检查" }));
    await waitFor(() => expect(importButton).toBeEnabled());
    fireEvent.click(screen.getByRole("button", { name: "刷新目录" }));
    await waitFor(() => expect(importButton).toBeDisabled());
  });
});
