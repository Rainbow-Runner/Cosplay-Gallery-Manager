import "@testing-library/jest-dom/vitest";

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { afterEach, describe, expect, it, vi } from "vitest";

import { messages } from "../i18n/messages";
import { ManageTagHierarchy } from "./ManageTagHierarchy";
import type { ManageTagTreeItem } from "./types";

const items: ManageTagTreeItem[] = [
  { uuid: "root-a", name: "Costume", aliases: [], parentUUIDs: [], metadataRevision: 2, childCount: 1, galleryCount: 0, allowDirectAssignment: false },
  { uuid: "root-b", name: "Portrait", aliases: [], parentUUIDs: [], metadataRevision: 2, childCount: 1, galleryCount: 0 },
  { uuid: "child", name: "Uniform", aliases: ["School outfit"], parentUUIDs: ["root-a", "root-b"], metadataRevision: 3, childCount: 0, galleryCount: 4 },
];

afterEach(cleanup);

function show(query = "") {
  const onSelect = vi.fn();
  const onCreateChild = vi.fn();
  render(<IntlProvider locale="en-GB" messages={messages["en-GB"]}><ManageTagHierarchy items={items} selected="" query={query} onSelect={onSelect} onCreateChild={onCreateChild} /></IntlProvider>);
  return { onSelect, onCreateChild };
}

describe("ManageTagHierarchy", () => {
  it("shows a multi-parent Tag beneath both expandable parents with one identity", () => {
    const { onSelect, onCreateChild } = show();
    expect(screen.queryByRole("button", { name: "Uniform" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Expand Costume" }));
    fireEvent.click(screen.getByRole("button", { name: "Expand Portrait" }));
    expect(screen.getAllByRole("button", { name: "Uniform" })).toHaveLength(2);
    fireEvent.click(screen.getAllByRole("button", { name: "Uniform" })[1]);
    expect(onSelect).toHaveBeenCalledWith("child");
    fireEvent.click(screen.getByRole("button", { name: "New child Tag under Costume" }));
    expect(onCreateChild).toHaveBeenCalledWith(items[0]);
  });

  it("finds an Alias and exposes both ancestor paths", () => {
    show("school");
    expect(screen.getAllByRole("button", { name: "Uniform" })).toHaveLength(2);
    expect(screen.getByRole("button", { name: "Costume" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Portrait" })).toBeInTheDocument();
  });

  it("marks category-only Tags without hiding them from the hierarchy", () => {
    show();
    expect(screen.getByRole("button", { name: "Costume" })).toBeInTheDocument();
    expect(screen.getByText("Category")).toBeInTheDocument();
  });
});
