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
});
