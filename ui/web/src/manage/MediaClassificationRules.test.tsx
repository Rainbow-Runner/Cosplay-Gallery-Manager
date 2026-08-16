import "@testing-library/jest-dom/vitest";

import { MockedProvider } from "@apollo/client/testing/react";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { MANAGE_MEDIA_CLASSIFICATION_RULES, MANAGE_MEDIA_CLASSIFICATION_SUGGESTIONS, VALIDATE_MEDIA_CLASSIFICATION_RULE } from "../api/manage";
import { MediaClassificationRules } from "./MediaClassificationRules";

afterEach(cleanup);

it("keeps Save disabled until the current RE2 expression is validated", async () => {
  const base = { libraryID: 2, name: "Selfie folders", enabled: true, order: 100, subject: "PARENT_FOLDER", operator: "RE2", caseSensitive: false, resultCategory: "SELFIE" };
  render(<MockedProvider mocks={[
    { request: { query: MANAGE_MEDIA_CLASSIFICATION_RULES, variables: { libraryID: 2 } }, result: { data: { manageMediaClassificationRules: [] } } },
    { request: { query: MANAGE_MEDIA_CLASSIFICATION_SUGGESTIONS, variables: { libraryID: 2, status: "PENDING" } }, result: { data: { manageMediaClassificationSuggestions: [] } } },
    { request: { query: VALIDATE_MEDIA_CLASSIFICATION_RULE, variables: { input: { ...base, pattern: "([" } } }, result: { data: { validateMediaClassificationRule: { valid: false, errorCode: "RULE_RE2_INVALID", message: "Invalid RE2 expression" } } } },
    { request: { query: VALIDATE_MEDIA_CLASSIFICATION_RULE, variables: { input: { ...base, pattern: "^selfie$" } } }, result: { data: { validateMediaClassificationRule: { valid: true, errorCode: "", message: "" } } } },
  ]}><MediaClassificationRules libraryID={2} /></MockedProvider>);
  fireEvent.click(await screen.findByText("Add classification rule"));
  const editor = screen.getByText("Add classification rule").closest("details") as HTMLElement;
  const fields = within(editor);
  fireEvent.change(fields.getByLabelText("Operator"), { target: { value: "RE2" } });
  fireEvent.change(fields.getByLabelText("RE2 expression"), { target: { value: "([" } });
  expect(fields.getByRole("button", { name: "Add rule" })).toBeDisabled();
  fireEvent.click(fields.getByRole("button", { name: "Validate RE2" }));
  expect(await fields.findByText(/RULE_RE2_INVALID/)).toBeInTheDocument();
  expect(fields.getByRole("button", { name: "Add rule" })).toBeDisabled();
  fireEvent.change(fields.getByLabelText("RE2 expression"), { target: { value: "^selfie$" } });
  expect(fields.getByText("Validation required before saving.")).toBeInTheDocument();
  fireEvent.click(fields.getByRole("button", { name: "Validate RE2" }));
  await waitFor(() => expect(fields.getByText("Valid RE2 expression.")).toBeInTheDocument());
  expect(fields.getByRole("button", { name: "Add rule" })).toBeEnabled();
});
