import { useMutation, useQuery } from "@apollo/client/react";
import { useIntl } from "react-intl";
import { useSearchParams } from "react-router-dom";
import { FAVORITE_GALLERIES, FAVORITE_MEDIA, GALLERY_HISTORY, SET_ITEM_FAVORITE } from "../api/browse";
import { GalleryCard } from "./GalleryCard";
import { RandomMediaCard } from "./RandomPage";
import { ScopeSelector } from "./ScopeSelector";
import type { GalleryPage, MediaPage, Scope } from "./types";

export function FavoritesPage() {
  const intl = useIntl(); const [parameters, setParameters] = useSearchParams(); const tab = parameters.get("tab") === "media" ? "media" : "galleries";
  const scope = (parameters.get("scope") as Scope) || "LIST"; const page = Math.max(1, Number(parameters.get("page")) || 1); const ratingSort = parameters.get("sort") === "rating";
  const galleries = useQuery<{ favoriteGalleries: GalleryPage }>(FAVORITE_GALLERIES, { variables: { scope, page }, skip: tab !== "galleries" });
  const media = useQuery<{ favoriteMedia: MediaPage }>(FAVORITE_MEDIA, { variables: { scope, page, ratingSort }, skip: tab !== "media" });
  const [setFavorite] = useMutation(SET_ITEM_FAVORITE);
  const update = (values: Record<string, string>) => setParameters({ tab, scope, page: "1", ...(ratingSort ? { sort: "rating" } : {}), ...values });
  return <main className="browse-main"><header className="page-heading"><p>{scope}</p><h1>{intl.formatMessage({ id: "page.favorites" })}</h1></header>
    <div className="personal-tabs"><button className={tab === "galleries" ? "is-active" : ""} onClick={() => update({ tab: "galleries" })}>{intl.formatMessage({ id: "favorites.galleries" })}</button>
      <button className={tab === "media" ? "is-active" : ""} onClick={() => update({ tab: "media" })}>{intl.formatMessage({ id: "favorites.media" })}</button></div>
    <ScopeSelector value={scope} onChange={(next) => update({ scope: next })} />
    {tab === "media" ? <label className="sort-toggle"><input type="checkbox" checked={ratingSort} onChange={(event) => update({ sort: event.target.checked ? "rating" : "favorite" })} /> {intl.formatMessage({ id: "favorites.ratingSort" })}</label> : null}
    {galleries.data ? <section className="gallery-grid">{galleries.data.favoriteGalleries.items.map((card) => <GalleryCard key={card.setID} card={card} scrubberEnabled={false} />)}</section> : null}
    {media.data ? <section className="random-grid">{media.data.favoriteMedia.items.map((item) => <RandomMediaCard key={item.itemUUID} item={item} onRemove={async () => {
      await setFavorite({ variables: { itemUUID: item.itemUUID, favorite: false } }); await media.refetch();
    }} />)}</section> : null}
  </main>;
}

export function HistoryPage() {
  const intl = useIntl(); const [parameters, setParameters] = useSearchParams(); const scope = (parameters.get("scope") as Scope) || "LIST"; const page = Math.max(1, Number(parameters.get("page")) || 1);
  const query = useQuery<{ galleryHistory: GalleryPage }>(GALLERY_HISTORY, { variables: { scope, page } });
  return <main className="browse-main"><header className="page-heading"><p>{scope}</p><h1>{intl.formatMessage({ id: "page.history" })}</h1></header>
    <ScopeSelector value={scope} onChange={(next) => setParameters({ scope: next, page: "1" })} />
    {query.data ? <section className="gallery-grid">{query.data.galleryHistory.items.map((card) => <GalleryCard key={card.setID} card={card} scrubberEnabled={false} />)}</section> : null}</main>;
}
