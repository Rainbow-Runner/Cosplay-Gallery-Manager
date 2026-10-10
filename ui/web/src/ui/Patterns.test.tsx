import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { DataTable, Dialog, Drawer, EmptyState, Pagination, Tabs } from "./Patterns";

describe("CGM design-system patterns", () => {
  it("closes the drawer with Escape and keeps close as the initial focus target", () => {
    const onClose = vi.fn();
    render(<Drawer open title="Primary navigation" closeLabel="Close navigation" onClose={onClose}><a href="/list">List</a></Drawer>);
    const dialog = screen.getByRole("dialog", { name: "Primary navigation" });
    expect(screen.getByRole("button", { name: "Close navigation" })).toHaveFocus();
    fireEvent.keyDown(dialog, { key: "Escape" });
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("exposes pagination and tab state through native accessibility attributes", () => {
    const onPageChange = vi.fn();
    const onTabChange = vi.fn();
    render(
      <>
        <Pagination page={2} totalPages={4} previousLabel="Previous" nextLabel="Next" onPageChange={onPageChange} />
        <Tabs label="Content type" active="gallery" items={[{ id: "gallery", label: "Galleries" }, { id: "media", label: "Media" }]} onChange={onTabChange} />
      </>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    fireEvent.click(screen.getByRole("tab", { name: "Media" }));
    expect(onPageChange).toHaveBeenCalledWith(3);
    expect(onTabChange).toHaveBeenCalledWith("media");
    expect(screen.getByRole("button", { name: "2" })).toHaveAttribute("aria-current", "page");
    expect(screen.getByRole("tab", { name: "Galleries" })).toHaveAttribute("aria-selected", "true");
  });

  it("keeps numeric pagination to a compact five-page window", () => {
    render(<Pagination page={5} totalPages={12} previousLabel="Previous" nextLabel="Next" onPageChange={vi.fn()} />);
    const pagination = screen.getAllByRole("navigation", { name: "Pagination" }).at(-1);
    expect(pagination).toBeDefined();
    expect(within(pagination!).getAllByRole("button").map((button) => button.textContent)).toEqual(["", "3", "4", "5", "6", "7", ""]);
  });

  it("provides semantic table and empty-state structures", () => {
    render(
      <>
        <DataTable label="Gallery issues"><tbody><tr><td>Missing cover</td></tr></tbody></DataTable>
        <EmptyState title="No galleries" />
      </>,
    );
    expect(screen.getByRole("table", { name: "Gallery issues" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "No galleries" })).toBeInTheDocument();
  });

  it("closes a dialog with Escape and restores focus after unmount", () => {
    const onClose = vi.fn();
    const trigger = document.createElement("button");
    document.body.append(trigger);
    trigger.focus();
    const { unmount } = render(
      <Dialog titleID="confirm-title" onClose={onClose}>
        <h2 id="confirm-title">Confirm action</h2>
        <input aria-label="Confirmation" autoFocus />
        <button type="button">Commit</button>
      </Dialog>,
    );
    const dialog = screen.getByRole("dialog", { name: "Confirm action" });
    expect(screen.getByLabelText("Confirmation")).toHaveFocus();
    fireEvent.keyDown(dialog, { key: "Escape" });
    expect(onClose).toHaveBeenCalledOnce();
    unmount();
    expect(trigger).toHaveFocus();
    trigger.remove();
  });
});
