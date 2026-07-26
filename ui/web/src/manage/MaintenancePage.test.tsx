import "@testing-library/jest-dom/vitest";

import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { MaintenancePage } from "./MaintenancePage";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("MaintenancePage", () => {
  it("requires one explicit map-or-disable decision plus password and MAP confirmation", async () => {
    const pending = {
      state: "WAITING_VALIDATION",
      requiresValidation: true,
      requiresPathMapping: true,
      libraries: [{ library_id: 7, name: "Archive", root_path: "D:\\Cosplay", enabled: true }],
    };
    const mapped = {
      state: "WAITING_VALIDATION",
      requiresValidation: true,
      requiresPathMapping: false,
      libraries: [],
    };
    const fetchMock = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(new Response(JSON.stringify(pending), { status: 200 }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
      .mockResolvedValueOnce(new Response(JSON.stringify(mapped), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    render(<MaintenancePage />);

    expect(await screen.findByText("Archive")).toBeInTheDocument();
    expect(screen.getByText("D:\\Cosplay")).toBeInTheDocument();
    const submit = screen.getByRole("button", { name: "Confirm path mappings" });
    expect(submit).toBeDisabled();
    fireEvent.click(screen.getByLabelText("Disable this library on this machine"));
    fireEvent.change(screen.getByLabelText("Owner password"), { target: { value: "owner secret" } });
    fireEvent.change(screen.getByLabelText(/Type MAP/), { target: { value: "MAP" } });
    expect(submit).toBeEnabled();
    fireEvent.click(submit);

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(3));
    const [url, options] = fetchMock.mock.calls[1]!;
    expect(url).toBe("/maintenance/path-mappings");
    expect(options?.method).toBe("POST");
    expect(JSON.parse(String(options?.body))).toEqual({
      mappings: [{ library_id: 7, expected_root_path: "D:\\Cosplay", root_path: "", disable: true }],
      password: "owner secret",
      confirmation: "MAP",
    });
    expect(await screen.findByText("Media library decisions saved. No media scan was started.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Validate environment and resume tasks" })).toBeEnabled();
  });
});
