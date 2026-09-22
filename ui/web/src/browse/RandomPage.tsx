import { useQuery } from "@apollo/client/react";
import { useState } from "react";
import { useIntl } from "react-intl";
import { Link, useNavigate } from "react-router-dom";
import { RANDOM_MEDIA } from "../api/browse";
import { itemResourceURL } from "./resourceUrl";
import { ScopeSelector } from "./ScopeSelector";
import type { RandomMediaItem, Scope } from "./types";

type Filter = "ALL" | "PHOTO" | "SELFIE" | "GIF" | "VIDEO";
export function RandomPage() {
  const intl = useIntl(); const [scope, setScope] = useState<Scope>("LIST"); const [filter, setFilter] = useState<Filter>("ALL");
  const { data, loading, error, refetch } = useQuery<{ randomMedia: RandomMediaItem[] }>(RANDOM_MEDIA, { variables: { scope, filter }, fetchPolicy: "network-only" });
  return <main className="browse-main"><header className="page-heading"><p>{scope}</p><h1>{intl.formatMessage({ id: "page.random" })}</h1></header>
    <div className="random-toolbar"><ScopeSelector value={scope} onChange={setScope} /><div className="media-filters">{(["ALL", "PHOTO", "SELFIE", "GIF", "VIDEO"] as Filter[]).map((value) =>
      <button key={value} type="button" className={filter === value ? "is-active" : ""} onClick={() => setFilter(value)}>{value}</button>)}</div>
      <button type="button" className="load-more" onClick={() => refetch()}>{intl.formatMessage({ id: "random.again" })}</button></div>
    {loading ? <p className="state-message">{intl.formatMessage({ id: "state.loading" })}</p> : null}
    {error ? <p className="state-message" role="alert">{intl.formatMessage({ id: "state.error" })}</p> : null}
    {data?.randomMedia.length ? <section className="random-grid">{data.randomMedia.map((item) => <RandomMediaCard key={item.itemUUID} item={item} />)}</section> : null}
  </main>;
}

export function RandomMediaCard({ item, onRemove }: { item: RandomMediaItem; onRemove?: () => void }) {
  const intl = useIntl(); const navigate = useNavigate();
  return <article className="random-card" onContextMenu={(event) => { event.preventDefault(); navigate(`/gallery/${item.gallerySlug}`); }}><Link className="random-card__poster" to={`/media/${item.itemUUID}`}>
    <img src={itemResourceURL(item.resource) ?? undefined} alt="" loading="lazy" />{item.mediaKind !== "STATIC_IMAGE" ? <span>{item.mediaKind === "VIDEO" ? "VIDEO" : "GIF"}</span> : null}</Link>
    <div className="random-card__body"><p>{item.characters.map((value) => value.name).join(" · ") || "\u00a0"}</p><p>{item.cosers.map((value) => value.name).join(" · ")}</p>
      <Link className="random-card__gallery" to={`/gallery/${item.gallerySlug}`} aria-label={intl.formatMessage({ id: "random.gallery" })}>•••</Link>
      {onRemove ? <button className="random-card__remove" type="button" onClick={onRemove} aria-label="Remove favourite">×</button> : null}</div></article>;
}
