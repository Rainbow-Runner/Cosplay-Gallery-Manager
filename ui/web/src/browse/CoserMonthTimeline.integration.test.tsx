import "@testing-library/jest-dom/vitest";
import { MockedProvider } from "@apollo/client/testing/react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { afterEach, expect, it, vi } from "vitest";
import { COSER_TIMELINE } from "../api/browse";
import { messages } from "../i18n/messages";
import { CoserMonthTimeline } from "./CoserMonthTimeline";

vi.mock("./GalleryCard", () => ({ GalleryCard: ({ card }: { card: { setID: string } }) => <article>{card.setID}</article> }));
afterEach(cleanup);

it("fills short viewports, retains cards after failure, retries and merges month boundaries", async () => {
  const request = (after: string | null) => ({ query: COSER_TIMELINE, variables: { coserUUID: "coser-1", scope: "ALL", date: "SHOOT", first: 24, after } });
  const entry = (month: string, setID: string) => ({ month, card: { setID } });
  const mocks = [
    { request: request(null), result: { data: { coserTimeline: { items: [entry("2026-10", "first")], endCursor: "next", hasNextPage: true } } } },
    { request: request("next"), error: new Error("offline") },
    { request: request("next"), result: { data: { coserTimeline: { items: [entry("2026-10", "first"), entry("2026-10", "second"), entry("2026-09", "older")], endCursor: "last", hasNextPage: false } } } },
  ];
  render(<IntlProvider locale="en-GB" messages={messages["en-GB"]}><MockedProvider mocks={mocks}><CoserMonthTimeline coserUUID="coser-1" scope="ALL" date="SHOOT" /></MockedProvider></IntlProvider>);
  expect(await screen.findByRole("alert")).toBeInTheDocument();
  expect(screen.getByText("first")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button"));
  expect(await screen.findByText("older")).toBeInTheDocument();
  expect(screen.getAllByText("first")).toHaveLength(1);
  expect(screen.getAllByRole("heading", { level: 2 })).toHaveLength(2);
  expect(screen.queryByRole("button")).not.toBeInTheDocument();
});
