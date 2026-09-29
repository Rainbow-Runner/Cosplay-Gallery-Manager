import "@testing-library/jest-dom/vitest";
import { MockedProvider } from "@apollo/client/testing/react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MANAGE_CORE_ENTITY_OPTIONS } from "../api/manage";
import { ManageEntitySelector } from "./ManageEntitySelector";

afterEach(cleanup);

function setup(kind: "COSER" | "CHARACTER" | "WORK" | "TAG" = "COSER", delay = 0) {
  const entity = { kind, uuid: "entity-1", name: "Alice", aliases: ["Alias"], workUUID: "", workName: "", metadataRevision: 1 };
  const select = vi.fn();
  render(<MockedProvider mocks={[{
    request: { query: MANAGE_CORE_ENTITY_OPTIONS, variables: { kind, query: "", limit: 20 } },
    result: { data: { manageCoreEntityOptions: [entity] } }, maxUsageCount: 5, delay,
  }]}><><ManageEntitySelector kind={kind} label={kind} uuid="" name="" onSelect={select} /><button>Outside</button></></MockedProvider>);
  const input = screen.getByRole("textbox", { name: `Search ${kind}` });
  fireEvent.focus(input);
  return { input, select, entity };
}

describe("ManageEntitySelector dismissal", () => {
  it.each(["COSER", "CHARACTER", "WORK", "TAG"] as const)("closes %s on outside pointer press without changing selection", async (kind) => {
    const { input, select } = setup(kind);
    await screen.findByRole("listbox");
    fireEvent.pointerDown(screen.getByRole("button", { name: "Outside" }));
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    expect(input).toHaveAttribute("aria-expanded", "false");
    expect(select).not.toHaveBeenCalled();
    fireEvent.focus(input);
    expect(await screen.findByRole("listbox")).toBeInTheDocument();
  });

  it("keeps inside clicks and focus transitions open until selection", async () => {
    const { input, select, entity } = setup();
    const option = await screen.findByRole("option");
    fireEvent.pointerDown(option);
    fireEvent.blur(input, { relatedTarget: option });
    expect(screen.getByRole("listbox")).toBeInTheDocument();
    fireEvent.click(option);
    expect(select).toHaveBeenCalledExactlyOnceWith(entity);
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  });

  it("closes with Escape, Tab-out and a press on the UUID field", async () => {
    const { input, select } = setup();
    await screen.findByRole("listbox");
    fireEvent.keyDown(input, { key: "Escape" });
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    fireEvent.click(input);
    await screen.findByRole("listbox");
    fireEvent.blur(input, { relatedTarget: screen.getByRole("button", { name: "Outside" }) });
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    fireEvent.focus(input);
    await screen.findByRole("listbox");
    fireEvent.pointerDown(screen.getByRole("textbox", { name: "COSER UUID" }));
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    expect(select).not.toHaveBeenCalled();
  });

  it("does not reopen when an outstanding query finishes after dismissal", async () => {
    const { input, select } = setup("COSER", 80);
    await screen.findByText("Searching…");
    fireEvent.pointerDown(screen.getByRole("button", { name: "Outside" }));
    await waitFor(() => expect(screen.queryByText("Searching…")).not.toBeInTheDocument());
    await new Promise((resolve) => window.setTimeout(resolve, 120));
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    expect(select).not.toHaveBeenCalled();
    fireEvent.focus(input);
    expect(await screen.findByRole("option")).toHaveTextContent("Alice");
  });
});
