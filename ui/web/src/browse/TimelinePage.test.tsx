import "@testing-library/jest-dom/vitest";

import { MockedProvider } from "@apollo/client/testing/react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it } from "vitest";

import { COSER_TIMELINE } from "../api/browse";
import { messages } from "../i18n/messages";
import { TimelinePage } from "./TimelinePage";

afterEach(cleanup);

describe("Coser timeline date modes", () => {
  it("defaults to all works and switches date source without losing scope", async () => {
    const page = { items: [], endCursor: "", hasNextPage: false };
    const mocks = (["SHOOT", "PUBLISH", "COMBINED"] as const).map((date) => ({
      request: { query: COSER_TIMELINE, variables: { scope: "ALL", first: 24, after: null, coserUUID: "coser-1", date } },
      result: { data: { coserTimeline: page } },
    }));
    render(<IntlProvider locale="en-GB" messages={messages["en-GB"]}><MockedProvider mocks={mocks}><MemoryRouter><TimelinePage coserUUID="coser-1" /></MemoryRouter></MockedProvider></IntlProvider>);
    expect(await screen.findByText("No galleries in this scope.")).toBeInTheDocument();
    const selector = screen.getByRole("combobox", { name: "Timeline date" });
    expect(selector).toHaveValue("SHOOT");
    fireEvent.change(selector, { target: { value: "PUBLISH" } });
    expect(selector).toHaveValue("PUBLISH");
    expect(await screen.findByText("No galleries in this scope.")).toBeInTheDocument();
    fireEvent.change(selector, { target: { value: "COMBINED" } });
    expect(selector).toHaveValue("COMBINED");
    expect(await screen.findByText("Uses publication date, then shoot date, then added date for each gallery.")).toBeInTheDocument();
  });
});
