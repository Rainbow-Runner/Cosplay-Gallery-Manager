import "@testing-library/jest-dom/vitest";

import { render, screen, within } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";

import { messages } from "../i18n/messages";
import { ManageHelpPage } from "./ManageHelpPage";

describe("ManageHelpPage", () => {
  it("explains both scan layers, explicit automation and Active Gallery handling", () => {
    render(<IntlProvider locale="en-GB" messages={messages["en-GB"]}><MemoryRouter><ManageHelpPage /></MemoryRouter></IntlProvider>);

    expect(screen.getByRole("heading", { name: "Help" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Libraries & import" })).toBeInTheDocument();
    expect(screen.getByText("Discovery scan", { selector: "th" })).toBeInTheDocument();
    expect(screen.getByText("Gallery source scan", { selector: "th" })).toBeInTheDocument();
    const activeRow = screen.getByText("Active Gallery", { selector: "th" }).closest("tr");
    expect(activeRow).not.toBeNull();
    expect(within(activeRow as HTMLElement).getByText(/does not rescan its source/)).toBeInTheDocument();
    expect(within(activeRow as HTMLElement).getByText(/reconciled by the scheduled source-scan phase/)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Open Libraries & import" })).toHaveAttribute("href", "/manage/libraries");
    expect(screen.getByText(/Saving a policy never starts a task/)).toBeInTheDocument();
  });

  it("explains Gallery Source and Manifest list statuses without changing the list", () => {
    render(<IntlProvider locale="zh-CN" messages={messages["zh-CN"]}><MemoryRouter><ManageHelpPage /></MemoryRouter></IntlProvider>);

    const guide = screen.getByRole("heading", { name: "作品集列表：Source 与 Manifest" }).closest("article");
    expect(guide).not.toBeNull();
    const section = within(guide as HTMLElement);
    expect(section.getByRole("link", { name: "打开作品集列表" })).toHaveAttribute("href", "/manage/gallery");
    expect(section.getByText("IN_SYNC", { selector: "th" })).toBeInTheDocument();
    expect(section.getByText("STALE", { selector: "th" })).toBeInTheDocument();
    expect(section.getByText("DB_DIRTY", { selector: "th" })).toBeInTheDocument();
    expect(section.getByText("FILE_DIRTY", { selector: "th" })).toBeInTheDocument();
    expect(section.getByText(/Source 的 IN_SYNC 与 Manifest 的 CLEAN 互相独立/)).toBeInTheDocument();
    expect(section.getByText(/STALE 本身不等于文件被改动或损坏/)).toBeInTheDocument();
    expect(section.getByText(/只跳过本地文件被改动的项目/)).toBeInTheDocument();
  });
});
