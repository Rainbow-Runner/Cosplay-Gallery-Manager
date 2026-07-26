import { useQuery } from "@apollo/client/react";
import { useIntl } from "react-intl";
import { useSearchParams } from "react-router-dom";

import { BROWSE_GALLERIES, BROWSE_UI_SETTINGS, HOME_GALLERIES } from "../api/browse";
import { GalleryCard } from "./GalleryCard";
import type { BrowseUISettings, GalleryPage, Scope } from "./types";

interface Props { scope?: Scope; home?: boolean }

export function GalleryIndexPage({ scope = "LIST", home = false }: Props) {
  const intl = useIntl();
  const [parameters] = useSearchParams();
  const page = Math.max(1, Number(parameters.get("page")) || 1);
  const query = useQuery<{ browseGalleries?: GalleryPage; homeGalleries?: { scope: Scope; page: GalleryPage } }>(
    home ? HOME_GALLERIES : BROWSE_GALLERIES,
    { variables: home ? { page } : { scope, page, sort: "RECENTLY_ADDED" } },
  );
  const settings = useQuery<{ browseUISettings: BrowseUISettings }>(BROWSE_UI_SETTINGS);
  const result = home ? query.data?.homeGalleries?.page : query.data?.browseGalleries;
  const effectiveScope = home ? query.data?.homeGalleries?.scope ?? scope : scope;
  const titleID = home ? "page.home" : effectiveScope === "MAGIC" ? "page.magic" : "page.list";

  return (
    <main className="browse-main">
      <header className="page-heading">
        <p>{effectiveScope}</p>
        <h1>{intl.formatMessage({ id: titleID })}</h1>
      </header>
      {query.loading ? <div className="gallery-grid gallery-grid--loading" aria-label={intl.formatMessage({ id: "state.loading" })} /> : null}
      {query.error ? <p className="state-message" role="alert">{intl.formatMessage({ id: "state.error" })}</p> : null}
      {result?.items.length ? (
        <section className="gallery-grid" aria-label={intl.formatMessage({ id: titleID })}>
          {result.items.map((card) => (
            <GalleryCard key={card.setID} card={card} scrubberEnabled={settings.data?.browseUISettings.galleryScrubberEnabled ?? false}
              favoriteControlVisible={settings.data?.browseUISettings.cardFavoriteControlVisible ?? false}
              ratingSummaryVisible={settings.data?.browseUISettings.cardRatingSummaryVisible ?? false} />
          ))}
        </section>
      ) : !query.loading && !query.error && !home ? <p className="state-message">{intl.formatMessage({ id: "state.empty" })}</p> : null}
      {result && result.totalPages > 1 ? (
        <nav className="pagination" aria-label={intl.formatMessage({ id: "pagination.label" })}>
          {page > 1 ? <a href={`?page=${page - 1}`}>{intl.formatMessage({ id: "pagination.previous" })}</a> : <span />}
          <span>{page} / {result.totalPages}</span>
          {page < result.totalPages ? <a href={`?page=${page + 1}`}>{intl.formatMessage({ id: "pagination.next" })}</a> : <span />}
        </nav>
      ) : null}
    </main>
  );
}
