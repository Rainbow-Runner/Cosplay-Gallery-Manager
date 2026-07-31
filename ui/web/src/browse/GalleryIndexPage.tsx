import { useQuery } from "@apollo/client/react";
import { useIntl } from "react-intl";
import { Link, useSearchParams } from "react-router-dom";

import { BROWSE_GALLERIES, BROWSE_UI_SETTINGS, HOME_GALLERIES } from "../api/browse";
import { Breadcrumbs, Pagination } from "../ui/Patterns";
import { GalleryCard } from "./GalleryCard";
import type { BrowseUISettings, CollectionType, GalleryPage, Scope } from "./types";

interface Props { scope?: Scope; collectionType?: CollectionType; titleID?: string; home?: boolean }

export function GalleryIndexPage({ scope = "LIST", collectionType, titleID: requestedTitleID, home = false }: Props) {
  const intl = useIntl();
  const [parameters, setParameters] = useSearchParams();
  const page = Math.max(1, Number(parameters.get("page")) || 1);
  const query = useQuery<{ browseGalleries?: GalleryPage; homeGalleries?: { scope: Scope; page: GalleryPage } }>(
    home ? HOME_GALLERIES : BROWSE_GALLERIES,
    { variables: home ? { page } : { scope, page, sort: "RECENTLY_ADDED", collectionType } },
  );
  const settings = useQuery<{ browseUISettings: BrowseUISettings }>(BROWSE_UI_SETTINGS);
  const result = home ? query.data?.homeGalleries?.page : query.data?.browseGalleries;
  const effectiveScope = home ? query.data?.homeGalleries?.scope ?? scope : scope;
  const titleID = requestedTitleID ?? (home ? "page.home" : effectiveScope === "MAGIC" ? "page.magic" : "page.list");
  const title = intl.formatMessage({ id: titleID });

  function setPage(next: number) {
    const updated = new URLSearchParams(parameters);
    updated.set("page", String(next));
    setParameters(updated);
  }

  return (
    <main className={`browse-main gallery-index-page${home ? " gallery-index-page--home" : ""}`}>
      {home ? <header className="page-heading"><h1>{title}</h1><p>{effectiveScope}</p></header> : <>
        <Breadcrumbs><li><Link to="/">{intl.formatMessage({ id: "nav.home" })}</Link></li><li aria-current="page">{title}</li></Breadcrumbs>
        <h1 className="sr-only">{title}</h1>
      </>}
      {query.loading ? <div className="gallery-grid gallery-grid--loading" aria-label={intl.formatMessage({ id: "state.loading" })} /> : null}
      {query.error ? <p className="state-message" role="alert">{intl.formatMessage({ id: "state.error" })}</p> : null}
      {result?.items.length ? (
        <section className="gallery-grid" aria-label={title}>
          {result.items.map((card) => (
            <GalleryCard key={card.setID} card={card} scrubberEnabled={settings.data?.browseUISettings.galleryScrubberEnabled ?? false}
              favoriteControlVisible={settings.data?.browseUISettings.cardFavoriteControlVisible ?? false}
              ratingSummaryVisible={settings.data?.browseUISettings.cardRatingSummaryVisible ?? false} />
          ))}
        </section>
      ) : !query.loading && !query.error && !home ? <p className="state-message">{intl.formatMessage({ id: "state.empty" })}</p> : null}
      {result && result.totalPages > 1 ? <Pagination page={page} totalPages={result.totalPages}
        previousLabel={intl.formatMessage({ id: "pagination.previous" })} nextLabel={intl.formatMessage({ id: "pagination.next" })} onPageChange={setPage} /> : null}
    </main>
  );
}
