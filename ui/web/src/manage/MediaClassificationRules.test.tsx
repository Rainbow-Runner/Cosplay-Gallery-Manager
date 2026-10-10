import "@testing-library/jest-dom/vitest";

import { MockedProvider } from "@apollo/client/testing/react";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { IntlProvider } from "react-intl";
import { MANAGE_MEDIA_CLASSIFICATION_RULES, MANAGE_MEDIA_CLASSIFICATION_SUGGESTIONS, VALIDATE_MEDIA_CLASSIFICATION_RULE } from "../api/manage";
import { messages } from "../i18n/messages";
import { MediaClassificationRules } from "./MediaClassificationRules";

afterEach(cleanup);

it("keeps Save disabled until the current RE2 expression is validated", async () => {
  const base = { libraryID: null, name: "Selfie folders", enabled: true, order: 100, subject: "PARENT_FOLDER", operator: "RE2", caseSensitive: false, resultCategory: "SELFIE" };
  render(<MockedProvider mocks={[
    { request: { query: MANAGE_MEDIA_CLASSIFICATION_RULES, variables: { libraryID: 2 } }, result: { data: { manageMediaClassificationRules: [] } } },
    { request: { query: MANAGE_MEDIA_CLASSIFICATION_SUGGESTIONS, variables: { libraryID: 2, status: "PENDING" } }, result: { data: { manageMediaClassificationSuggestions: [] } } },
    { request: { query: VALIDATE_MEDIA_CLASSIFICATION_RULE, variables: { input: { ...base, pattern: "([" } } }, result: { data: { validateMediaClassificationRule: { valid: false, errorCode: "RULE_RE2_INVALID", message: "Invalid RE2 expression" } } } },
    { request: { query: VALIDATE_MEDIA_CLASSIFICATION_RULE, variables: { input: { ...base, pattern: "^selfie$" } } }, result: { data: { validateMediaClassificationRule: { valid: true, errorCode: "", message: "" } } } },
  ]}><IntlProvider locale="en-GB" messages={messages["en-GB"]}><MediaClassificationRules library={{ id: 2, name: "Collection" }} /></IntlProvider></MockedProvider>);
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

it("separates global rules from selected-library overrides and defaults new rules to global", async () => {
  const rules = [
    { __typename: "ManageMediaClassificationRule", id: 1, libraryID: null, name: "Global selfies", enabled: true, order: 100, subject: "PARENT_FOLDER", operator: "EXACT", pattern: "selfie", caseSensitive: false, resultCategory: "SELFIE", revision: 1, systemDefault: true },
    { __typename: "ManageMediaClassificationRule", id: 2, libraryID: 2, name: "Collection exception", enabled: true, order: 100, subject: "FILE_NAME", operator: "GLOB", pattern: "portrait*", caseSensitive: false, resultCategory: "PHOTO", revision: 1, systemDefault: false },
  ];
  render(<MockedProvider mocks={[
    { request: { query: MANAGE_MEDIA_CLASSIFICATION_RULES, variables: { libraryID: 2 } }, result: { data: { manageMediaClassificationRules: rules } } },
    { request: { query: MANAGE_MEDIA_CLASSIFICATION_SUGGESTIONS, variables: { libraryID: 2, status: "PENDING" } }, result: { data: { manageMediaClassificationSuggestions: [] } } },
  ]}><IntlProvider locale="en-GB" messages={messages["en-GB"]}><MediaClassificationRules library={{ id: 2, name: "Collection" }} /></IntlProvider></MockedProvider>);

  await waitFor(() => expect(screen.getByRole("region", { name: "Global rules" })).toHaveTextContent("Global selfies"));
  expect(screen.getByRole("region", { name: "Overrides for Collection" })).toHaveTextContent("Collection exception");
  fireEvent.click(screen.getByText("Add classification rule"));
  expect(screen.getByLabelText("Rule scope")).toHaveValue("GLOBAL");
  expect(screen.getByText(/Effective rules for Collection = global rules \+ overrides/)).toBeInTheDocument();
});

it("renders the classification workflow in Simplified Chinese", async () => {
  render(<MockedProvider mocks={[
    { request: { query: MANAGE_MEDIA_CLASSIFICATION_RULES, variables: { libraryID: 2 } }, result: { data: { manageMediaClassificationRules: [] } } },
    { request: { query: MANAGE_MEDIA_CLASSIFICATION_SUGGESTIONS, variables: { libraryID: 2, status: "PENDING" } }, result: { data: { manageMediaClassificationSuggestions: [] } } },
  ]}><IntlProvider locale="zh-CN" messages={messages["zh-CN"]}><MediaClassificationRules library={{ id: 2, name: "收藏库" }} /></IntlProvider></MockedProvider>);

  expect(screen.getByRole("heading", { name: "媒体分类规则" })).toBeInTheDocument();
  expect(screen.getByRole("region", { name: "全局规则" })).toBeInTheDocument();
  expect(screen.getByRole("region", { name: "“收藏库”的覆盖规则" })).toBeInTheDocument();
  fireEvent.click(screen.getByText("添加分类规则"));
  expect(screen.getByLabelText("规则作用范围")).toHaveValue("GLOBAL");
  expect(screen.getByDisplayValue("自拍文件夹")).toBeInTheDocument();
});
