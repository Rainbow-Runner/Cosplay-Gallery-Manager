import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, describe, expect, it } from "vitest";

import { messages } from "../i18n/messages";
import { BrowseShell } from "./BrowseShell";

afterEach(cleanup);

function renderShell() {
  return render(
    <IntlProvider locale="en-GB" messages={messages["en-GB"]}>
      <MemoryRouter initialEntries={["/"]}>
        <Routes>
          <Route element={<BrowseShell />}>
            <Route index element={<p>Home content</p>} />
            <Route path="list" element={<p>List content</p>} />
          </Route>
        </Routes>
      </MemoryRouter>
    </IntlProvider>,
  );
}

describe("BrowseShell navigation", () => {
  it("groups every existing Browse route in the fixed navigation", () => {
    const { container } = renderShell();
    const sidebar = container.querySelector<HTMLElement>(".browse-sidebar");
    expect(sidebar).not.toBeNull();
    expect(within(sidebar!).getByRole("link", { name: "Home" })).toHaveClass("active");
    expect(within(sidebar!).getByRole("link", { name: "History" })).toHaveAttribute("href", "/history");
    expect(within(sidebar!).getByText("Galleries")).toBeInTheDocument();
    expect(within(sidebar!).getByText("Library")).toBeInTheDocument();
    expect(within(sidebar!).getByText("Explore")).toBeInTheDocument();
  });

  it("opens the mobile drawer, navigates, closes it and restores menu focus", async () => {
    renderShell();
    const menu = screen.getByRole("button", { name: "Open navigation" });
    fireEvent.click(menu);
    const dialog = screen.getByRole("dialog", { name: "Primary navigation" });
    fireEvent.click(within(dialog).getByRole("link", { name: "List" }));
    expect(await screen.findByText("List content")).toBeInTheDocument();
    expect(screen.queryByRole("dialog", { name: "Primary navigation" })).not.toBeInTheDocument();
    expect(menu).toHaveFocus();
  });

  it("closes the mobile drawer with Escape", () => {
    renderShell();
    const menu = screen.getByRole("button", { name: "Open navigation" });
    fireEvent.click(menu);
    fireEvent.keyDown(screen.getByRole("dialog", { name: "Primary navigation" }), { key: "Escape" });
    expect(screen.queryByRole("dialog", { name: "Primary navigation" })).not.toBeInTheDocument();
    expect(menu).toHaveFocus();
  });
});
