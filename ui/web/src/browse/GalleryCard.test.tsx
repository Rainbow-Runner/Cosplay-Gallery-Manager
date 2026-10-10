import { MockedProvider } from "@apollo/client/testing/react";
import { cleanup, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it } from "vitest";

import { GalleryCard } from "./GalleryCard";
import type { BrowseGalleryCard } from "./types";

afterEach(cleanup);

const card = {
  setID: "gallery-1",
  slug: "saber-at-the-lake",
  title: "Saber at the lake",
  collectionType: "COSPLAY",
  contentRating: "NON_ADULT",
  cover: { kind: "GENERATED", revision: 1, managed: true, warning: false },
  credits: [{ uuid: "coser-alice", name: "Alice", avatarURL: "/resource/coser/coser-alice/2/avatar-480" }, { uuid: "coser-bob", name: "Bob", avatarURL: null }],
  creditCount: 2,
  characters: [{ uuid: "character-saber", name: "Saber" }],
  characterCount: 1,
  works: [{ uuid: "work-fate", name: "Fate/stay night" }],
  workCount: 1,
  addedAtUTC: "2026-07-30T00:00:00Z",
  media: { photo: 2, selfie: 0, gif: 0, video: 0 },
  favorite: false,
  scrubberCount: 0,
  scrubberRevision: 0,
} satisfies BrowseGalleryCard;

describe("GalleryCard Coser links", () => {
  it("links every displayed Coser avatar and name to the existing canonicalising detail route", () => {
    render(
      <MockedProvider>
        <MemoryRouter>
          <GalleryCard card={card} scrubberEnabled={false} />
        </MemoryRouter>
      </MockedProvider>,
    );

    expect(screen.getByRole("link", { name: "Alice" })).toHaveAttribute("href", "/coser/coser-alice");
    expect(screen.getByRole("link", { name: "Bob" })).toHaveAttribute("href", "/coser/coser-bob");
    expect(screen.getByRole("link", { name: "Alice" }).querySelector("img")).toHaveAttribute("src", "/resource/coser/coser-alice/2/avatar-480");
  });

  it("does not render an empty Coser link for an Album without credits", () => {
    render(
      <MockedProvider>
        <MemoryRouter>
          <GalleryCard card={{ ...card, collectionType: "ALBUM", credits: [], creditCount: 0 }} scrubberEnabled={false} />
        </MemoryRouter>
      </MockedProvider>,
    );

    expect(screen.queryByRole("link", { name: "Alice" })).not.toBeInTheDocument();
  });

  it("links Album credits through the Model presentation route", () => {
    render(
      <MockedProvider>
        <MemoryRouter>
          <GalleryCard card={{ ...card, collectionType: "ALBUM" }} scrubberEnabled={false} />
        </MemoryRouter>
      </MockedProvider>,
    );

    expect(screen.getByRole("link", { name: "Alice" })).toHaveAttribute("href", "/model/coser-alice");
    expect(screen.getByRole("link", { name: "Bob" })).toHaveAttribute("href", "/model/coser-bob");
  });
});
