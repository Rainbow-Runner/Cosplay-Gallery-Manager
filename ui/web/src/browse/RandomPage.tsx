import { useQuery } from "@apollo/client/react";
import { useState } from "react";
import { useIntl } from "react-intl";
import { Link, Navigate, useNavigate, useSearchParams } from "react-router-dom";
import { BROWSE_UI_SETTINGS, RANDOM_GALLERIES, RANDOM_MEDIA } from "../api/browse";
import { GalleryCard } from "./GalleryCard";
import { itemResourceURL } from "./resourceUrl";
import { ScopeSelector } from "./ScopeSelector";
import type { BrowseUISettings, BrowseGalleryCard, RandomMediaItem, Scope } from "./types";

type Filter = "ALL" | "PHOTO" | "SELFIE" | "GIF" | "VIDEO";
export function RandomPage({ mode = "media" }: { mode?: "media" | "galleries" }) {
  const intl = useIntl(); const [parameters] = useSearchParams();
  if (mode === "media" && parameters.get("mode") === "galleries") return <Navigate to="/random-galleries" replace />;
  return <main className="browse-main"><header className="page-heading"><h1>{intl.formatMessage({ id: mode === "galleries" ? "random.galleries" : "page.random" })}</h1></header>
    {mode === "galleries" ? <RandomGalleryBrowser /> : <RandomMediaBrowser />}
  </main>;
}

function RandomGalleryBrowser() {
  const intl = useIntl();
  const { data, loading, error, refetch } = useQuery<{ randomGalleries: BrowseGalleryCard[] }>(RANDOM_GALLERIES, { fetchPolicy: "no-cache", notifyOnNetworkStatusChange: true });
  const settings = useQuery<{ browseUISettings: BrowseUISettings }>(BROWSE_UI_SETTINGS);
  return <>
    <div className="random-gallery-toolbar"><p>{intl.formatMessage({ id: "random.galleriesHelp" })}</p><button type="button" className="load-more" disabled={loading} onClick={() => refetch()}>{intl.formatMessage({ id: "random.again" })}</button></div>
    {loading ? <p className="state-message">{intl.formatMessage({ id: "state.loading" })}</p> : null}
    {error ? <p className="state-message" role="alert">{intl.formatMessage({ id: "state.error" })}</p> : null}
    {data?.randomGalleries.length ? <section className="gallery-grid" aria-label={intl.formatMessage({ id: "random.galleries" })}>{data.randomGalleries.map((card) => <GalleryCard key={card.setID} card={card}
      scrubberEnabled={settings.data?.browseUISettings.galleryScrubberEnabled ?? false}
      favoriteControlVisible={settings.data?.browseUISettings.cardFavoriteControlVisible ?? false}
      ratingSummaryVisible={settings.data?.browseUISettings.cardRatingSummaryVisible ?? false} />)}</section> : !loading && !error ? <p className="state-message">{intl.formatMessage({ id: "state.empty" })}</p> : null}
  </>;
}

function RandomMediaBrowser() {
  const intl = useIntl(); const [scope, setScope] = useState<Scope>("LIST"); const [filter, setFilter] = useState<Filter>("ALL");
  const { data, loading, error, refetch } = useQuery<{ randomMedia: RandomMediaItem[] }>(RANDOM_MEDIA, { variables: { scope, filter }, fetchPolicy: "network-only" });
  return <>
    <div className="random-toolbar"><ScopeSelector value={scope} onChange={setScope} /><div className="media-filters">{(["ALL", "PHOTO", "SELFIE", "GIF", "VIDEO"] as Filter[]).map((value) =>
      <button key={value} type="button" className={filter === value ? "is-active" : ""} onClick={() => setFilter(value)}>{value}</button>)}</div>
      <button type="button" className="load-more" onClick={() => refetch()}>{intl.formatMessage({ id: "random.again" })}</button></div>
    {loading ? <p className="state-message">{intl.formatMessage({ id: "state.loading" })}</p> : null}
    {error ? <p className="state-message" role="alert">{intl.formatMessage({ id: "state.error" })}</p> : null}
    {data?.randomMedia.length ? <section className="random-grid">{data.randomMedia.map((item) => <RandomMediaCard key={item.itemUUID} item={item} />)}</section> : null}
  </>;
}

export function RandomMediaCard({ item, onRemove }: { item: RandomMediaItem; onRemove?: () => void }) {
  const intl = useIntl(); const navigate = useNavigate();
  return <article className="random-card" onContextMenu={(event) => { event.preventDefault(); navigate(`/gallery/${item.gallerySlug}`); }}><Link className="random-card__poster" to={`/media/${item.itemUUID}`}>
    <img src={itemResourceURL(item.resource) ?? undefined} alt="" loading="lazy" />{item.mediaKind !== "STATIC_IMAGE" ? <span>{item.mediaKind === "VIDEO" ? "VIDEO" : "GIF"}</span> : null}</Link>
    <div className="random-card__body"><p>{item.characters.map((value) => value.name).join(" · ") || "\u00a0"}</p><p>{item.cosers.map((value) => value.name).join(" · ")}</p>
      <Link className="random-card__gallery" to={`/gallery/${item.gallerySlug}`} aria-label={intl.formatMessage({ id: "random.gallery" })}>•••</Link>
      {onRemove ? <button className="random-card__remove" type="button" onClick={onRemove} aria-label="Remove favourite">×</button> : null}</div></article>;
}
