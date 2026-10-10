import "@testing-library/jest-dom/vitest";

import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { afterEach, describe, expect, it, vi } from "vitest";

import { LoginPage } from "./LoginPage";

afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

describe("LoginPage owner recovery", () => {
  it("requires a token and matching new passwords without putting secrets in the URL", async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true });
    vi.stubGlobal("fetch", fetchMock);
    render(<IntlProvider locale="en-GB" messages={{ "login.forgot": "Forgot password?", "login.recoveryTitle": "Recover owner password", "login.recoveryToken": "Recovery token", "login.newPassword": "New password", "setup.confirmPassword": "Confirm password", "login.resetPassword": "Reset password", "login.recoverySuccess": "Password reset.", "login.title": "Owner login", "setup.password": "Password", "login.submit": "Sign in", "nav.legal": "Legal" }}><LoginPage /></IntlProvider>);
    fireEvent.click(screen.getByRole("button", { name: "Forgot password?" }));
    const submit = screen.getByRole("button", { name: "Reset password" });
    expect(submit).toBeDisabled();
    fireEvent.change(screen.getByLabelText("Recovery token"), { target: { value: "x".repeat(43) } });
    fireEvent.change(screen.getByLabelText("New password"), { target: { value: "new strong password" } });
    fireEvent.change(screen.getByLabelText("Confirm password"), { target: { value: "new strong password" } });
    fireEvent.click(submit);
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith("/session/recover", expect.objectContaining({ method: "POST" })));
    expect(window.location.pathname).not.toContain("x".repeat(43));
    expect(await screen.findByText("Password reset.")).toBeInTheDocument();
  });
});
