import "@testing-library/jest-dom/vitest";

import { MockedProvider } from "@apollo/client/testing/react";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { IntlProvider } from "react-intl";
import { MANAGE_MEDIA_EXCLUSION_DECISIONS, MANAGE_MEDIA_EXCLUSION_RULES, VALIDATE_MEDIA_EXCLUSION_RULE } from "../api/manage";
import { messages } from "../i18n/messages";
import { MediaExclusionRules } from "./MediaExclusionRules";

afterEach(cleanup);

it("requires server validation before saving an RE2 exclusion rule", async () => {
  const base = { libraryID: null, name: "Excluded media", enabled: true, order: 100, subject: "PARENT_FOLDER", operator: "RE2", caseSensitive: false, mediaKind: "ALL", decision: "EXCLUDE" };
  render(<MockedProvider mocks={[
    { request: { query: MANAGE_MEDIA_EXCLUSION_RULES, variables: { libraryID: 2 } }, result: { data: { manageMediaExclusionRules: [] } } },
    { request: { query: MANAGE_MEDIA_EXCLUSION_DECISIONS, variables: { libraryID: 2, status: "PENDING" } }, result: { data: { manageMediaExclusionDecisions: [] } } },
    { request: { query: VALIDATE_MEDIA_EXCLUSION_RULE, variables: { input: { ...base, pattern: "^extras$" } } }, result: { data: { validateMediaExclusionRule: { valid: true, errorCode: "", message: "" } } } },
  ]}><IntlProvider locale="en-GB" messages={messages["en-GB"]}><MediaExclusionRules library={{ id: 2, name: "Collection" }} /></IntlProvider></MockedProvider>);
  fireEvent.click(await screen.findByText("Add automatic exclusion rule"));
  const editor = screen.getByText("Add automatic exclusion rule").closest("details") as HTMLElement;
  const fields = within(editor);
  fireEvent.change(fields.getByLabelText("Operator"), { target: { value: "RE2" } });
  fireEvent.change(fields.getByLabelText("RE2 expression"), { target: { value: "^extras$" } });
  expect(fields.getByRole("button", { name: "Add rule" })).toBeDisabled();
  fireEvent.click(fields.getByRole("button", { name: "Validate RE2" }));
  await waitFor(() => expect(fields.getByText("Valid RE2 expression.")).toBeInTheDocument());
  expect(fields.getByRole("button", { name: "Add rule" })).toBeEnabled();
});

it("separates global exclusion policy from library INCLUDE exceptions", async () => {
  const rules = [
    { __typename: "ManageMediaExclusionRule", id: 1, libraryID: null, name: "Global extras", enabled: true, order: 100, subject: "PARENT_FOLDER", operator: "EXACT", pattern: "extras", caseSensitive: false, mediaKind: "ALL", decision: "EXCLUDE", revision: 1, systemDefault: false },
    { __typename: "ManageMediaExclusionRule", id: 2, libraryID: 2, name: "Keep covers", enabled: true, order: 100, subject: "FILE_NAME", operator: "GLOB", pattern: "cover.*", caseSensitive: false, mediaKind: "STATIC_IMAGE", decision: "INCLUDE", revision: 1, systemDefault: false },
  ];
  render(<MockedProvider mocks={[
    { request: { query: MANAGE_MEDIA_EXCLUSION_RULES, variables: { libraryID: 2 } }, result: { data: { manageMediaExclusionRules: rules } } },
    { request: { query: MANAGE_MEDIA_EXCLUSION_DECISIONS, variables: { libraryID: 2, status: "PENDING" } }, result: { data: { manageMediaExclusionDecisions: [] } } },
  ]}><IntlProvider locale="en-GB" messages={messages["en-GB"]}><MediaExclusionRules library={{ id: 2, name: "Collection" }} /></IntlProvider></MockedProvider>);

  await waitFor(() => expect(screen.getByRole("region", { name: "Global exclusion policy" })).toHaveTextContent("Global extras"));
  expect(screen.getByRole("region", { name: "Exclusion overrides for Collection" })).toHaveTextContent("Keep covers");
  expect(screen.getByRole("region", { name: "Exclusion overrides for Collection" })).toHaveTextContent("INCLUDE exception");
  fireEvent.click(within(screen.getByRole("region", { name: "Global exclusion policy" })).getByRole("button", { name: "Delete" }));
  expect(within(screen.getByRole("region", { name: "Global exclusion policy" })).getByRole("button", { name: "Confirm delete" })).toBeInTheDocument();
  fireEvent.click(screen.getByText("Add automatic exclusion rule"));
  expect(screen.getByLabelText("Rule scope")).toHaveValue("GLOBAL");
});

it("renders the automatic exclusion workflow in Simplified Chinese", async () => {
  render(<MockedProvider mocks={[
    { request: { query: MANAGE_MEDIA_EXCLUSION_RULES, variables: { libraryID: 2 } }, result: { data: { manageMediaExclusionRules: [] } } },
    { request: { query: MANAGE_MEDIA_EXCLUSION_DECISIONS, variables: { libraryID: 2, status: "PENDING" } }, result: { data: { manageMediaExclusionDecisions: [] } } },
  ]}><IntlProvider locale="zh-CN" messages={messages["zh-CN"]}><MediaExclusionRules library={{ id: 2, name: "收藏库" }} /></IntlProvider></MockedProvider>);

  expect(screen.getByRole("heading", { name: "自动排除规则" })).toBeInTheDocument();
  expect(screen.getByRole("region", { name: "全局排除策略" })).toBeInTheDocument();
  expect(screen.getByRole("region", { name: "“收藏库”的排除覆盖规则" })).toBeInTheDocument();
  fireEvent.click(screen.getByText("添加自动排除规则"));
  expect(screen.getByLabelText("规则作用范围")).toHaveValue("GLOBAL");
  expect(screen.getByDisplayValue("排除媒体")).toBeInTheDocument();
});
