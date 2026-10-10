import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { AppLogo } from "./AppLogo";
import { Icon } from "./Icon";
import { Alert, Badge, Button, Field, Input } from "./Primitives";

describe("CGM design-system primitives", () => {
  it("renders the original local wordmark and line icon without a remote asset", () => {
    render(<><AppLogo subtitle="Collection" /><Icon name="search" label="Search icon" /></>);
    expect(screen.getByText("Cosplay Gallery Manager")).toBeInTheDocument();
    expect(screen.getByText("Collection")).toBeInTheDocument();
    expect(screen.getByRole("img", { name: "Search icon" })).toHaveAttribute("viewBox", "0 0 24 24");
  });

  it("exposes button state and preserves native disabled behavior", () => {
    const onClick = vi.fn();
    render(<Button variant="primary" disabled onClick={onClick}>Save</Button>);
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(onClick).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Save" })).toHaveClass("cgm-button--primary");
  });

  it("keeps form controls and status messages accessible", () => {
    render(
      <>
        <Field label="Title"><Input required /></Field>
        <Badge>DRAFT</Badge>
        <Alert>Saved locally</Alert>
        <Alert danger>Activation blocked</Alert>
      </>,
    );
    expect(screen.getByRole("textbox", { name: "Title" })).toBeRequired();
    expect(screen.getByRole("status")).toHaveTextContent("Saved locally");
    expect(screen.getByRole("alert")).toHaveTextContent("Activation blocked");
  });
});
