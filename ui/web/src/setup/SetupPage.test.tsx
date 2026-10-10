import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import { SetupPage } from "./SetupPage";

afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

describe("SetupPage Docker defaults", () => {
  it("uses server-declared environment and opens first-time setup directly", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ ok: true, json: async () => ({ complete: false, runtimeEnvironment: "DOCKER", coserMetadataRoot: "/var/lib/cgm/cosers", backupRoot: "/var/lib/cgm/backups" }) }));
    render(<MemoryRouter><IntlProvider locale="en-GB" messages={{ "setup.title": "Setup", "setup.environment": "Environment", "setup.firstRunAccess": "The first person to complete setup becomes the owner.", "setup.next": "Continue", "setup.back": "Back", "setup.authentication": "Authentication", "setup.password": "Password", "setup.confirmPassword": "Confirm password" }}><SetupPage /></IntlProvider></MemoryRouter>);
    expect(await screen.findByText("The first person to complete setup becomes the owner.")).toBeInTheDocument();
    expect(screen.queryByLabelText("One-time terminal ticket")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Continue" })).toBeEnabled();
  });
});
