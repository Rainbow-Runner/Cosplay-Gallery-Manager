import { useMutation, useQuery } from "@apollo/client/react";
import { useEffect, useMemo, useRef, useState } from "react";
import { useIntl } from "react-intl";
import { Link, useNavigate, useParams } from "react-router-dom";

import { BROWSE_UI_SETTINGS, GALLERY_DETAIL, GALLERY_MEMBER_INDEX, RECORD_GALLERY_VIEW, RELATED_GALLERIES } from "../api/browse";
import { GalleryCard } from "./GalleryCard";
import { itemResourceURL } from "./resourceUrl";
import type { BrowseUISettings, GalleryDetail, GalleryMember, GalleryMemberIndex } from "./types";
import { Breadcrumbs } from "../ui/Patterns";
import { Icon } from "../ui/Icon";

const memberBatchSize = 24;
type MediaFilter = "ALL" | "PHOTO" | "SELFIE" | "GIF" | "VIDEO";

export function visualMemberGroups(items: GalleryMember[], filter: MediaFilter) {
  const staticItems = items.filter((item) => item.mediaKind === "STATIC_IMAGE" && (filter === "ALL" || item.imageCategory === filter));
  const gifItems = items.filter((item) => item.mediaKind === "ANIMATED_IMAGE" && (filter === "ALL" || filter === "GIF"));
  const videoItems = items.filter((item) => item.mediaKind === "VIDEO" && (filter === "ALL" || filter === "VIDEO"));
  return [
    { key: "photo", titleID: "gallery.group.photo", items: staticItems },
    { key: "gif", titleID: "gallery.group.gif", items: gifItems },
    { key: "video", titleID: "gallery.group.video", items: videoItems },
  ].filter((group) => group.items.length > 0);
}

export function lightboxNavigationState(index: number, total: number) {
  return { canPrevious: index > 0, canNext: index >= 0 && index < total - 1 };
}

export function GalleryDetailPage() {
  const { slug = "" } = useParams();
  const navigate = useNavigate();
  const intl = useIntl();
  const detailQuery = useQuery<{ galleryDetail: GalleryDetail }>(GALLERY_DETAIL, { variables: { slug } });
  const settingsQuery = useQuery<{ browseUISettings: BrowseUISettings }>(BROWSE_UI_SETTINGS);
  const detail = detailQuery.data?.galleryDetail;
  const memberQuery = useQuery<{ galleryMemberIndex: GalleryMemberIndex }>(GALLERY_MEMBER_INDEX, {
    variables: { setID: detail?.card.setID ?? "" },
    skip: !detail,
  });
  const relatedQuery = useQuery<{ relatedGalleries: Array<{ card: GalleryDetail["card"] }> }>(RELATED_GALLERIES, {
    variables: { setID: detail?.card.setID ?? "" },
    skip: !detail,
  });
  const [filter, setFilter] = useState<MediaFilter>("ALL");
  const [visible, setVisible] = useState<Record<string, number>>({ photo: memberBatchSize, gif: memberBatchSize, video: memberBatchSize });
  const [lightboxIndex, setLightboxIndex] = useState<number | null>(null);
  const [recordView] = useMutation(RECORD_GALLERY_VIEW);

  useEffect(() => {
    if (detail?.redirected && detail.card.slug !== slug) navigate(`/gallery/${detail.card.slug}`, { replace: true });
  }, [detail, navigate, slug]);
  useEffect(() => {
    setFilter("ALL");
    setVisible({ photo: memberBatchSize, gif: memberBatchSize, video: memberBatchSize });
    setLightboxIndex(null);
  }, [detail?.card.setID]);
  useEffect(() => {
    if (detail?.card.setID) void recordView({ variables: { setID: detail.card.setID, itemUUID: null } });
  }, [detail?.card.setID, recordView]);

  const members = memberQuery.data?.galleryMemberIndex.items ?? [];
  const groups = useMemo(() => visualMemberGroups(members, filter), [filter, members]);
  const lightboxItem = lightboxIndex === null ? null : members[lightboxIndex];
  const lightboxNavigation = lightboxNavigationState(lightboxIndex ?? -1, members.length);

  if (detailQuery.loading) return <main className="browse-main"><p className="state-message">{intl.formatMessage({ id: "state.loading" })}</p></main>;
  if (detailQuery.error || !detail) return <main className="browse-main"><p className="state-message" role="alert">{intl.formatMessage({ id: "state.error" })}</p></main>;

  const coverURL = detail.card.cover.resource ? itemResourceURL(detail.card.cover.resource) : "";
  const settings = settingsQuery.data?.browseUISettings;
  return (
    <main className="gallery-detail">
      <Breadcrumbs>
        <li><Link to="/">Home</Link></li>
        <li aria-current="page">{detail.card.title}</li>
      </Breadcrumbs>
      <section className="gallery-hero">
        <div className="gallery-hero__cover">{coverURL ? <img src={coverURL} alt="" /> : <span>CGM</span>}</div>
        <div className="gallery-hero__body">
          <p className="gallery-hero__kind">{detail.card.collectionType}</p>
          <h1>{detail.card.title}</h1>
          <div className="gallery-hero__dates">
            {detail.card.shootDate ? <time title={intl.formatMessage({ id: "gallery.shootDate" })} aria-label={`${intl.formatMessage({ id: "gallery.shootDate" })}: ${detail.card.shootDate}`}>◷ {detail.card.shootDate}</time> : null}
            <time className="gallery-hero__added" title={intl.formatMessage({ id: "gallery.addedAt" })} aria-label={`${intl.formatMessage({ id: "gallery.addedAt" })}: ${detail.card.addedAtUTC}`}>＋ {new Date(detail.card.addedAtUTC).toLocaleDateString()}</time>
          </div>
          <div className="gallery-hero__relations">
            {detail.credits.map((credit) => (
              <div key={credit.coser.uuid}>
                <Link to={`/coser/${credit.coser.uuid}`}>{credit.coser.name}</Link>
                {credit.characters.map((character) => <Link key={character.uuid} to={`/character/${character.uuid}`}>{character.name}</Link>)}
              </div>
            ))}
          </div>
          {detail.description ? <p className="gallery-description">{detail.description}</p> : null}
          <div className="tag-row">{detail.tags.map((tag) => <Link key={tag.uuid} to={`/tag/${tag.uuid}`}>#{tag.name}</Link>)}</div>
          <dl className="gallery-facts">
            {detail.photographerName ? <><dt>{intl.formatMessage({ id: "gallery.photographer" })}</dt><dd>{detail.photographerName}</dd></> : null}
            {detail.studioName ? <><dt>{intl.formatMessage({ id: "gallery.studio" })}</dt><dd>{detail.studioName}</dd></> : null}
            <dt>{intl.formatMessage({ id: "gallery.size" })}</dt><dd>{formatBytes(detail.availableBytes)}</dd>
          </dl>
        </div>
      </section>

      <div className="gallery-detail__columns"><section className="gallery-members">
        <header className="section-heading"><h2>{intl.formatMessage({ id: "gallery.contents" })}</h2><span>{members.length}</span></header>
        {settings?.detailMediaFilterEnabled ? (
          <div className="media-filters" role="group" aria-label={intl.formatMessage({ id: "gallery.filter" })}>
            {(["ALL", "PHOTO", "SELFIE", "GIF", "VIDEO"] as MediaFilter[]).map((value) => (
              <button key={value} type="button" className={filter === value ? "is-active" : ""} onClick={() => setFilter(value)}>{value}</button>
            ))}
          </div>
        ) : null}
        {memberQuery.loading ? <p className="state-message">{intl.formatMessage({ id: "state.loading" })}</p> : null}
        {groups.map((group) => (
          <section className="media-group" key={group.key}>
            <header><h3>{intl.formatMessage({ id: group.titleID })}</h3><span>{group.items.length}</span></header>
            <div className="media-grid">
              {group.items.slice(0, visible[group.key]).map((item) => {
                const fullIndex = members.findIndex((value) => value.itemUUID === item.itemUUID);
                return <MediaTile key={item.itemUUID} item={item} onOpen={() => { setLightboxIndex(fullIndex); void recordView({ variables: { setID: detail.card.setID, itemUUID: item.itemUUID } }); }} />;
              })}
            </div>
            {visible[group.key] < group.items.length ? (
              <button className="load-more" type="button" onClick={() => setVisible((current) => ({ ...current, [group.key]: current[group.key] + memberBatchSize }))}>
                {intl.formatMessage({ id: "gallery.loadMore" })}
              </button>
            ) : null}
          </section>
        ))}
      </section>

      {relatedQuery.data?.relatedGalleries.length ? (
        <section className="related-section">
          <header className="section-heading"><h2>{intl.formatMessage({ id: "gallery.related" })}</h2></header>
          <div className="gallery-grid">{relatedQuery.data.relatedGalleries.map(({ card }) => <GalleryCard key={card.setID} card={card} scrubberEnabled={settings?.galleryScrubberEnabled ?? false} />)}</div>
        </section>
      ) : null}</div>

      {detail.externalLinks.length ? (
        <footer className="external-links">{detail.externalLinks.map((link) => <a key={link.uuid} href={link.url} rel="noreferrer" target="_blank">↗ {link.label || link.type}</a>)}</footer>
      ) : null}

      {lightboxItem ? (
        <Lightbox item={lightboxItem} onClose={() => setLightboxIndex(null)}
          canPrevious={lightboxNavigation.canPrevious} canNext={lightboxNavigation.canNext}
          onPrevious={() => { if (lightboxNavigation.canPrevious) setLightboxIndex(lightboxIndex! - 1); }}
          onNext={() => { if (lightboxNavigation.canNext) setLightboxIndex(lightboxIndex! + 1); }} />
      ) : null}
    </main>
  );
}

function MediaTile({ item, onOpen }: { item: GalleryMember; onOpen: () => void }) {
  const resource = item.cardResource;
  return (
    <button className="media-tile" type="button" onClick={onOpen} aria-label={item.caption || item.imageCategory || item.mediaKind}>
      {resource ? <img src={itemResourceURL(resource) ?? undefined} alt="" loading="lazy" /> : <span className="media-tile__pending">{item.processingState}</span>}
      {item.mediaKind !== "STATIC_IMAGE" ? <span className="media-tile__kind">{item.mediaKind === "VIDEO" ? "VIDEO" : "GIF"}</span> : null}
      {item.caption ? <span className="media-tile__caption">{item.caption}</span> : null}
    </button>
  );
}

function Lightbox({ item, onClose, canPrevious, canNext, onPrevious, onNext }: {
  item: GalleryMember;
  onClose: () => void;
  canPrevious: boolean;
  canNext: boolean;
  onPrevious: () => void;
  onNext: () => void;
}) {
  const resource = item.largeResource ?? item.cardResource;
  const closeRef = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    closeRef.current?.focus();
    return () => { document.body.style.overflow = previousOverflow; };
  }, []);
  return (
    <div className="lightbox" role="dialog" aria-modal="true" aria-label={item.caption || item.mediaKind} onClick={onClose}
      onKeyDown={(event) => { if (event.key === "Escape") onClose(); else if (event.key === "ArrowLeft" && canPrevious) onPrevious(); else if (event.key === "ArrowRight" && canNext) onNext(); }}>
      <button ref={closeRef} className="lightbox__close" type="button" aria-label="Close media viewer" onClick={(event) => { event.stopPropagation(); onClose(); }}><Icon name="close" /></button>
      <button className="lightbox__previous" type="button" aria-label="Previous media" disabled={!canPrevious} onClick={(event) => { event.stopPropagation(); onPrevious(); }}><Icon name="chevron-left" /></button>
      <div className="lightbox__content" onClick={(event) => event.stopPropagation()}>
        {resource ? <img src={itemResourceURL(resource) ?? undefined} alt={item.caption} /> : <span>{item.processingState}</span>}
        {item.caption ? <p>{item.caption}</p> : null}
      </div>
      <button className="lightbox__next" type="button" aria-label="Next media" disabled={!canNext} onClick={(event) => { event.stopPropagation(); onNext(); }}><Icon name="chevron-right" /></button>
    </div>
  );
}

function formatBytes(value: number) {
  if (value < 1024) return `${value} B`;
  const units = ["KB", "MB", "GB", "TB"];
  let amount = value;
  let unit = -1;
  do { amount /= 1024; unit += 1; } while (amount >= 1024 && unit < units.length - 1);
  return `${amount.toFixed(amount >= 10 ? 1 : 2)} ${units[unit]}`;
}
