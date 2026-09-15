import { skipToken, useMutation, useQuery } from "@apollo/client/react";
import { useEffect, useMemo, useRef, useState } from "react";
import { useIntl } from "react-intl";
import { Link, useNavigate, useParams, useSearchParams } from "react-router-dom";

import {
  BROWSE_UI_SETTINGS,
  GALLERY_DETAIL,
  GALLERY_MEMBER_INDEX,
  RECORD_GALLERY_VIEW,
  RELATED_GALLERIES,
  SET_BROWSE_GALLERY_COVER_ITEM,
  SET_GALLERY_FAVORITE,
  SET_ITEM_FAVORITE,
  SET_ITEM_RATING,
} from "../api/browse";
import { Breadcrumbs } from "../ui/Patterns";
import { Icon } from "../ui/Icon";
import { formatGalleryMediaCount } from "./galleryMediaCount";
import { galleryCardPresentation } from "./galleryCardPresentation";
import { GalleryTagEditor } from "./GalleryTagEditor";
import { itemResourceURL } from "./resourceUrl";
import type { BrowseGalleryCard, BrowseUISettings, GalleryDetail, GalleryMember, GalleryMemberIndex } from "./types";
import { useImageDisplay } from "./useImageDisplay";
import { useOnDemandAnimatedPreview } from "./useOnDemandAnimatedPreview";
import { useOnDemandVideoPlayback } from "./useOnDemandVideoPlayback";

const memberBatchSize = 24;
const animationHoverConfirmationMS = 150;
const defaultAnimatedPlaybackLimit = 12;
const defaultAnimatedLockIntervalMS = 800;
type MediaFilter = "ALL" | "PHOTO" | "SELFIE" | "GIF" | "VIDEO";

export function visualMemberGroups(items: GalleryMember[], filter: MediaFilter) {
  const staticItems = items.filter((item) => item.mediaKind === "STATIC_IMAGE" && (filter === "ALL" || item.imageCategory === filter));
  const gifItems = items.filter((item) => item.mediaKind === "ANIMATED_IMAGE" && (filter === "ALL" || filter === "GIF"));
  const videoItems = items.filter((item) => item.mediaKind === "VIDEO" && (filter === "ALL" || filter === "VIDEO"));
  return [
    { key: "photo", items: staticItems },
    { key: "gif", items: gifItems },
    { key: "video", items: videoItems },
  ].filter((group) => group.items.length > 0);
}

export function lightboxNavigationState(index: number, total: number) {
  return { canPrevious: index > 0, canNext: index >= 0 && index < total - 1 };
}

export function animationPlaybackWindow(orderedUUIDs: string[], anchorUUID: string, limit: number) {
	if (orderedUUIDs.length <= limit) return [...orderedUUIDs];
	const anchorIndex = Math.max(0, orderedUUIDs.indexOf(anchorUUID));
	const leftCount = Math.floor((limit - 1) / 2);
	const start = Math.min(Math.max(0, anchorIndex - leftCount), orderedUUIDs.length - limit);
	return orderedUUIDs.slice(start, start + limit);
}

export function animatedPlaybackSelection(playbackWindow: string[], visibleUUIDs: Set<string>, reducedMotion: boolean) {
  if (reducedMotion) return [];
  return playbackWindow.filter((uuid) => visibleUUIDs.has(uuid));
}

export function animationLockDelay(now: number, lastLockAt: number, lockIntervalMS: number) {
	if (lastLockAt <= 0) return animationHoverConfirmationMS;
	return Math.max(animationHoverConfirmationMS, lastLockAt + lockIntervalMS - now);
}

export function GalleryDetailPage() {
  const { slug = "" } = useParams();
  const navigate = useNavigate();
  const [parameters, setParameters] = useSearchParams();
  const intl = useIntl();
  const detailQuery = useQuery<{ galleryDetail: GalleryDetail }>(GALLERY_DETAIL, { variables: { slug } });
  const settingsQuery = useQuery<{ browseUISettings: BrowseUISettings }>(BROWSE_UI_SETTINGS, { fetchPolicy: "cache-and-network" });
  const detail = detailQuery.data?.galleryDetail;
  const memberQuery = useQuery<{ galleryMemberIndex: GalleryMemberIndex }>(GALLERY_MEMBER_INDEX,
    detail ? { variables: { setID: detail.card.setID } } : skipToken);
  const relatedQuery = useQuery<{ relatedGalleries: Array<{ card: BrowseGalleryCard }> }>(RELATED_GALLERIES,
    detail ? { variables: { setID: detail.card.setID } } : skipToken);
  const [recordView] = useMutation(RECORD_GALLERY_VIEW);
  const [saveGalleryFavorite] = useMutation<{ setGalleryFavorite: { metadataRevision: number } }>(SET_GALLERY_FAVORITE);
  const [saveItemFavorite] = useMutation<{ setItemFavorite: { metadataRevision: number } }>(SET_ITEM_FAVORITE);
  const [saveItemRating] = useMutation<{ setItemRating: { metadataRevision: number } }>(SET_ITEM_RATING);
  const [saveCover] = useMutation<{ setGalleryCoverItem: { row: { metadataRevision: number } } }>(SET_BROWSE_GALLERY_COVER_ITEM);

  const [filter, setFilter] = useState<MediaFilter>("ALL");
  const [visible, setVisible] = useState<Record<string, number>>({ photo: memberBatchSize, gif: memberBatchSize, video: memberBatchSize });
  const [galleryFavorite, setGalleryFavorite] = useState(false);
  const [favoriteOverrides, setFavoriteOverrides] = useState<Record<string, boolean>>({});
  const [ratingOverrides, setRatingOverrides] = useState<Record<string, number | null>>({});
  const [metadataRevision, setMetadataRevision] = useState(0);
  const [coverItemUUID, setCoverItemUUID] = useState("");
  const [galleryTags, setGalleryTags] = useState<GalleryDetail["tags"]>([]);
  const [busyItemUUID, setBusyItemUUID] = useState("");
  const [actionMessage, setActionMessage] = useState("");
  const [moreDetailsOpen, setMoreDetailsOpen] = useState(false);
  const [reducedMotion, setReducedMotion] = useState(() => typeof window !== "undefined" && typeof window.matchMedia === "function" && window.matchMedia("(prefers-reduced-motion: reduce)").matches);
  const [hoverCapable, setHoverCapable] = useState(() => typeof window !== "undefined" && typeof window.matchMedia === "function" && window.matchMedia("(hover: hover) and (pointer: fine)").matches);
  const [visibleAnimatedUUIDs, setVisibleAnimatedUUIDs] = useState<Set<string>>(() => new Set());
  const [animatedPlaybackWindow, setAnimatedPlaybackWindow] = useState<string[]>([]);
  const openedHere = useRef(false);
  const previouslyOpenItem = useRef<string | null>(null);
  const tileElements = useRef(new Map<string, HTMLElement>());
  const moreDetailsRef = useRef<HTMLDetailsElement>(null);
  const animationHoverTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const animationHoverCandidate = useRef("");
  const lastAnimationLockAt = useRef(0);

  useEffect(() => {
    if (!detail?.redirected || detail.card.slug === slug) return;
    const query = parameters.toString();
    navigate(`/gallery/${detail.card.slug}${query ? `?${query}` : ""}`, { replace: true });
  }, [detail, navigate, parameters, slug]);
  useEffect(() => {
    setFilter("ALL");
    setVisible({ photo: memberBatchSize, gif: memberBatchSize, video: memberBatchSize });
    setFavoriteOverrides({});
    setRatingOverrides({});
    setActionMessage("");
    setMoreDetailsOpen(false);
    openedHere.current = false;
  }, [detail?.card.setID]);
  useEffect(() => {
    if (!detail) return;
    setGalleryFavorite(detail.card.favorite);
    setCoverItemUUID(detail.card.cover.resource?.itemUUID ?? "");
    setGalleryTags(detail.tags);
    setMetadataRevision(detail.metadataRevision);
  }, [detail]);
  useEffect(() => {
    if (memberQuery.data) setMetadataRevision(memberQuery.data.galleryMemberIndex.metadataRevision);
  }, [memberQuery.data]);
  useEffect(() => {
    if (detail?.card.setID) void recordView({ variables: { setID: detail.card.setID, itemUUID: null } });
  }, [detail?.card.setID, recordView]);
  useEffect(() => {
    if (!moreDetailsOpen) return;
    const closeOutside = (event: PointerEvent) => {
      if (!moreDetailsRef.current?.contains(event.target as Node)) setMoreDetailsOpen(false);
    };
    const closeWithEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") setMoreDetailsOpen(false);
    };
    document.addEventListener("pointerdown", closeOutside);
    document.addEventListener("keydown", closeWithEscape);
    return () => {
      document.removeEventListener("pointerdown", closeOutside);
      document.removeEventListener("keydown", closeWithEscape);
    };
  }, [moreDetailsOpen]);

  useEffect(() => {
    if (typeof window.matchMedia !== "function") return;
    const query = window.matchMedia("(prefers-reduced-motion: reduce)");
    const update = () => setReducedMotion(query.matches);
    update();
    query.addEventListener?.("change", update);
    return () => query.removeEventListener?.("change", update);
  }, []);
  useEffect(() => {
    if (typeof window.matchMedia !== "function") return;
    const query = window.matchMedia("(hover: hover) and (pointer: fine)");
    const update = () => setHoverCapable(query.matches);
    update();
    query.addEventListener?.("change", update);
    return () => query.removeEventListener?.("change", update);
  }, []);

  const members = memberQuery.data?.galleryMemberIndex.items ?? [];
  const groups = useMemo(() => visualMemberGroups(members, filter), [filter, members]);
  const navigationItems = useMemo(() => groups.flatMap((group) => group.items), [groups]);
  const orderedAnimatedUUIDs = useMemo(() => navigationItems.filter((item) => item.mediaKind === "ANIMATED_IMAGE").map((item) => item.itemUUID), [navigationItems]);
  const animatedPlaybackLimit = settingsQuery.data?.browseUISettings.galleryAnimatedPlaybackLimit ?? defaultAnimatedPlaybackLimit;
  const animatedLockIntervalMS = settingsQuery.data?.browseUISettings.galleryAnimatedLockIntervalMS ?? defaultAnimatedLockIntervalMS;
  const requestedItemUUID = parameters.get("item");
  const lightboxIndex = requestedItemUUID ? navigationItems.findIndex((item) => item.itemUUID === requestedItemUUID) : -1;
  const lightboxItem = lightboxIndex >= 0 ? navigationItems[lightboxIndex] : null;
  const lightboxNavigation = lightboxNavigationState(lightboxIndex, navigationItems.length);
  const displayedAnimatedUUIDs = useMemo(() => groups.flatMap((group) => group.items.slice(0, visible[group.key]))
    .filter((item) => item.mediaKind === "ANIMATED_IMAGE").map((item) => item.itemUUID), [groups, visible]);
  const displayedAnimatedKey = displayedAnimatedUUIDs.join("|");
  const orderedAnimatedKey = orderedAnimatedUUIDs.join("|");

  useEffect(() => {
    if (animationHoverTimer.current) clearTimeout(animationHoverTimer.current);
    animationHoverTimer.current = null;
    animationHoverCandidate.current = "";
    lastAnimationLockAt.current = 0;
    setAnimatedPlaybackWindow(orderedAnimatedUUIDs.slice(0, animatedPlaybackLimit));
  // Stable joined identity avoids resetting a deliberate lock for unrelated renders.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [animatedPlaybackLimit, orderedAnimatedKey]);

  useEffect(() => {
    if (reducedMotion || lightboxItem || typeof IntersectionObserver === "undefined") {
      setVisibleAnimatedUUIDs(new Set());
      return;
    }
    const visibleItems = new Set<string>();
    const publish = () => {
      setVisibleAnimatedUUIDs((current) => current.size === visibleItems.size && [...visibleItems].every((uuid) => current.has(uuid)) ? current : new Set(visibleItems));
    };
    const observer = new IntersectionObserver((entries) => {
      for (const entry of entries) {
        const uuid = (entry.target as HTMLElement).dataset.itemUuid;
        if (!uuid) continue;
        if (entry.isIntersecting) visibleItems.add(uuid); else visibleItems.delete(uuid);
      }
      publish();
    }, { threshold: 0.15 });
    for (const uuid of displayedAnimatedUUIDs) {
      const element = tileElements.current.get(uuid);
      if (element) observer.observe(element);
    }
    return () => observer.disconnect();
  // The joined identity is intentional: it avoids rebuilding the observer for
  // unrelated object identity changes while keeping displayed order stable.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [displayedAnimatedKey, lightboxItem, reducedMotion]);

  useEffect(() => {
    if (hoverCapable || orderedAnimatedUUIDs.length <= animatedPlaybackLimit || visibleAnimatedUUIDs.size === 0) return;
    const visibleInOrder = orderedAnimatedUUIDs.filter((uuid) => visibleAnimatedUUIDs.has(uuid));
    if (!visibleInOrder.length) return;
    const anchor = visibleInOrder[Math.floor((visibleInOrder.length - 1) / 2)];
    const next = animationPlaybackWindow(orderedAnimatedUUIDs, anchor, animatedPlaybackLimit);
    setAnimatedPlaybackWindow((current) => current.length === next.length && next.every((uuid, index) => current[index] === uuid) ? current : next);
  }, [animatedPlaybackLimit, hoverCapable, orderedAnimatedUUIDs, visibleAnimatedUUIDs]);

  useEffect(() => () => {
    if (animationHoverTimer.current) clearTimeout(animationHoverTimer.current);
  }, []);

  const activeAnimatedUUIDs = useMemo(() => new Set(animatedPlaybackSelection(animatedPlaybackWindow, visibleAnimatedUUIDs, reducedMotion || Boolean(lightboxItem))),
    [animatedPlaybackWindow, lightboxItem, reducedMotion, visibleAnimatedUUIDs]);
  const animationHoverEnabled = hoverCapable && !reducedMotion && orderedAnimatedUUIDs.length > animatedPlaybackLimit;

  function beginAnimationHover(itemUUID: string) {
    if (!animationHoverEnabled) return;
    if (animationHoverTimer.current) clearTimeout(animationHoverTimer.current);
    animationHoverCandidate.current = itemUUID;
    const now = Date.now();
    animationHoverTimer.current = setTimeout(() => {
      animationHoverTimer.current = null;
      if (animationHoverCandidate.current !== itemUUID) return;
      const next = animationPlaybackWindow(orderedAnimatedUUIDs, itemUUID, animatedPlaybackLimit);
      setAnimatedPlaybackWindow((current) => {
        if (current.length === next.length && next.every((uuid, index) => current[index] === uuid)) return current;
        lastAnimationLockAt.current = Date.now();
        return next;
      });
    }, animationLockDelay(now, lastAnimationLockAt.current, animatedLockIntervalMS));
  }

  function endAnimationHover(itemUUID: string) {
    if (animationHoverCandidate.current !== itemUUID) return;
    animationHoverCandidate.current = "";
    if (animationHoverTimer.current) clearTimeout(animationHoverTimer.current);
    animationHoverTimer.current = null;
  }

  function revealItem(itemUUID: string) {
    const group = visualMemberGroups(members, filter).find((candidate) => candidate.items.some((item) => item.itemUUID === itemUUID));
    if (!group) return;
    const required = group.items.findIndex((item) => item.itemUUID === itemUUID) + 1;
    setVisible((current) => current[group.key] >= required ? current : { ...current, [group.key]: required });
    requestAnimationFrame(() => requestAnimationFrame(() => tileElements.current.get(itemUUID)?.scrollIntoView?.({ block: "center" })));
  }

  useEffect(() => {
    if (requestedItemUUID && lightboxItem && detail) {
      if (previouslyOpenItem.current !== requestedItemUUID) {
        previouslyOpenItem.current = requestedItemUUID;
        void recordView({ variables: { setID: detail.card.setID, itemUUID: requestedItemUUID } });
      }
      return;
    }
    if (!requestedItemUUID && previouslyOpenItem.current) {
      const closedItem = previouslyOpenItem.current;
      previouslyOpenItem.current = null;
      openedHere.current = false;
      revealItem(closedItem);
    }
  }, [detail, lightboxItem, recordView, requestedItemUUID]);

  if (detailQuery.loading) return <main className="browse-main"><p className="state-message">{intl.formatMessage({ id: "state.loading" })}</p></main>;
  if (detailQuery.error || !detail) return <main className="browse-main"><p className="state-message" role="alert">{intl.formatMessage({ id: "state.error" })}</p></main>;

  const settings = settingsQuery.data?.browseUISettings;
  const setID = detail.card.setID;
  const mediaCount = formatGalleryMediaCount(detail.card.media);
  const personalControlsVisible = settings?.detailRatingControlVisible ?? false;

  function openItem(itemUUID: string) {
    openedHere.current = true;
    const next = new URLSearchParams(parameters);
    next.set("item", itemUUID);
    setParameters(next);
  }
  function moveLightbox(index: number) {
    const item = navigationItems[index];
    if (!item) return;
    const next = new URLSearchParams(parameters);
    next.set("item", item.itemUUID);
    setParameters(next, { replace: true });
  }
  function closeLightbox() {
    if (openedHere.current) {
      navigate(-1);
      return;
    }
    const next = new URLSearchParams(parameters);
    next.delete("item");
    setParameters(next, { replace: true });
  }
  async function toggleGalleryFavorite() {
    const next = !galleryFavorite;
    setGalleryFavorite(next);
    try {
      await saveGalleryFavorite({ variables: { setID, favorite: next } });
    } catch {
      setGalleryFavorite(!next);
      setActionMessage(intl.formatMessage({ id: "gallery.actionFailed" }));
    }
  }
  async function toggleItemFavorite(item: GalleryMember) {
    const current = favoriteOverrides[item.itemUUID] ?? item.favorite;
    const next = !current;
    setFavoriteOverrides((values) => ({ ...values, [item.itemUUID]: next }));
    try {
      const result = await saveItemFavorite({ variables: { itemUUID: item.itemUUID, favorite: next } });
      if (result.data) setMetadataRevision(result.data.setItemFavorite.metadataRevision);
    } catch {
      setFavoriteOverrides((values) => ({ ...values, [item.itemUUID]: current }));
      setActionMessage(intl.formatMessage({ id: "gallery.actionFailed" }));
    }
  }
  async function rateItem(item: GalleryMember, rating: number | null) {
    const current = ratingOverrides[item.itemUUID] ?? item.ratingHalfSteps ?? null;
    setRatingOverrides((values) => ({ ...values, [item.itemUUID]: rating }));
    setBusyItemUUID(item.itemUUID);
    try {
      const result = await saveItemRating({ variables: { itemUUID: item.itemUUID, ratingHalfSteps: rating, expectedMetadataRevision: metadataRevision } });
      if (result.data) setMetadataRevision(result.data.setItemRating.metadataRevision);
    } catch {
      setRatingOverrides((values) => ({ ...values, [item.itemUUID]: current }));
      setActionMessage(intl.formatMessage({ id: "gallery.actionFailed" }));
    } finally {
      setBusyItemUUID("");
    }
  }
  async function setItemAsCover(item: GalleryMember) {
    if (item.mediaKind !== "STATIC_IMAGE" || item.itemUUID === coverItemUUID) return;
    const previous = coverItemUUID;
    setCoverItemUUID(item.itemUUID);
    setBusyItemUUID(item.itemUUID);
    setActionMessage("");
    try {
      const result = await saveCover({ variables: { setID, itemUUID: item.itemUUID, expectedMetadataRevision: metadataRevision } });
      if (result.data) setMetadataRevision(result.data.setGalleryCoverItem.row.metadataRevision);
      setActionMessage(intl.formatMessage({ id: "gallery.coverUpdated" }));
      await Promise.all([detailQuery.refetch(), memberQuery.refetch()]);
    } catch {
      setCoverItemUUID(previous);
      setActionMessage(intl.formatMessage({ id: "gallery.actionFailed" }));
    } finally {
      setBusyItemUUID("");
    }
  }

  return (
    <main className="gallery-detail">
      <Breadcrumbs>
        <li><Link to="/">Home</Link></li>
        <li aria-current="page">{detail.card.title}</li>
      </Breadcrumbs>

      <header className="gallery-detail__header">
        <div className="gallery-detail__title">
          <h1>{detail.card.title}</h1>
          <div className="gallery-detail__stats">
            {mediaCount ? <span>{mediaCount}</span> : null}
            {detail.card.shootDate ? <time title={intl.formatMessage({ id: "gallery.shootDate" })} aria-label={`${intl.formatMessage({ id: "gallery.shootDate" })}: ${detail.card.shootDate}`}><Icon name="calendar" />{detail.card.shootDate}</time> : null}
            <time className="gallery-detail__added" title={intl.formatMessage({ id: "gallery.addedAt" })} aria-label={`${intl.formatMessage({ id: "gallery.addedAt" })}: ${detail.card.addedAtUTC}`}>＋ {new Date(detail.card.addedAtUTC).toLocaleDateString()}</time>
          </div>
        </div>
        <div className="gallery-detail__actions">
          {settings?.cardFavoriteControlVisible ? <button className={galleryFavorite ? "is-active" : ""} type="button" aria-label={intl.formatMessage({ id: galleryFavorite ? "gallery.unfavorite" : "gallery.favorite" })} aria-pressed={galleryFavorite} onClick={toggleGalleryFavorite}><Icon name="heart" /></button> : null}
          <details ref={moreDetailsRef} className="gallery-detail__more" open={moreDetailsOpen}>
            <summary onClick={(event) => { event.preventDefault(); setMoreDetailsOpen((open) => !open); }}><Icon name="info" />{intl.formatMessage({ id: "gallery.moreDetails" })}</summary>
            <div className="gallery-detail__metadata">
              <p className="gallery-detail__kind">{detail.card.collectionType}</p>
              <div className="gallery-detail__relations">
                {detail.credits.map((credit) => (
                  <div key={credit.coser.uuid}>
                    <Link to={`/${detail.card.collectionType === "ALBUM" ? "model" : "coser"}/${credit.coser.uuid}`}>{credit.coser.name}</Link>
                    {credit.works.map((work) => <Link key={work.uuid} to={`/work/${work.uuid}`}>{work.name}</Link>)}
                    {credit.characters.map((character) => <Link key={character.uuid} to={`/character/${character.uuid}`}>{character.name}</Link>)}
                  </div>
                ))}
              </div>
              {detail.description ? <p className="gallery-description">{detail.description}</p> : null}
              <GalleryTagEditor setID={setID} metadataRevision={metadataRevision} tags={galleryTags} onSaved={(tags, revision) => {
                setGalleryTags(tags);
                setMetadataRevision(revision);
                void relatedQuery.refetch();
              }} />
              <dl className="gallery-facts">
                {detail.photographerName ? <><dt>{intl.formatMessage({ id: "gallery.photographer" })}</dt><dd>{detail.photographerName}</dd></> : null}
                {detail.studioName ? <><dt>{intl.formatMessage({ id: "gallery.studio" })}</dt><dd>{detail.studioName}</dd></> : null}
                <dt>{intl.formatMessage({ id: "gallery.size" })}</dt><dd>{formatBytes(detail.availableBytes)}</dd>
                {detail.mediaParentDirectories.length ? <><dt>{intl.formatMessage({ id: "gallery.mediaFolders" })}</dt><dd><ul className="gallery-detail__directories">{detail.mediaParentDirectories.map((directory) => <li key={directory}><code>{directory}</code></li>)}</ul></dd></> : null}
              </dl>
              {detail.externalLinks.length ? <div className="external-links">{detail.externalLinks.map((link) => <a key={link.uuid} href={link.url} rel="noreferrer" target="_blank">↗ {link.label || link.type}</a>)}</div> : null}
              <Link className="gallery-detail__manage" to={`/manage/gallery/${encodeURIComponent(detail.card.setID)}?tab=media`}><Icon name="settings" />{intl.formatMessage({ id: "gallery.manage" })}</Link>
            </div>
          </details>
        </div>
      </header>
      {actionMessage ? <p className="gallery-detail__message" role="status">{actionMessage}</p> : null}

      <div className="gallery-detail__columns">
        <section className="gallery-members" aria-label={intl.formatMessage({ id: "gallery.contents" })}>
          {settings?.detailMediaFilterEnabled ? (
            <div className="media-filters" role="group" aria-label={intl.formatMessage({ id: "gallery.filter" })}>
              {(["ALL", "PHOTO", "SELFIE", "GIF", "VIDEO"] as MediaFilter[]).map((value) => (
                <button key={value} type="button" className={filter === value ? "is-active" : ""} onClick={() => setFilter(value)}>{value}</button>
              ))}
            </div>
          ) : null}
          {memberQuery.loading ? <p className="state-message">{intl.formatMessage({ id: "state.loading" })}</p> : null}
          <div className="media-sequence">
            {groups.map((group) => (
              <section className="media-group" key={group.key} aria-label={group.key}>
                <div className="media-grid">
                  {group.items.slice(0, visible[group.key]).map((item) => (
                    <MediaTile
                      key={item.itemUUID}
                      item={item}
                      animate={activeAnimatedUUIDs.has(item.itemUUID)}
                      favorite={favoriteOverrides[item.itemUUID] ?? item.favorite}
                      currentCover={coverItemUUID === item.itemUUID}
                      personalControlsVisible={personalControlsVisible}
                      busy={busyItemUUID === item.itemUUID}
                      setElement={(element) => { if (element) tileElements.current.set(item.itemUUID, element); else tileElements.current.delete(item.itemUUID); }}
                      onOpen={() => openItem(item.itemUUID)}
                      onAnimationHoverStart={animationHoverEnabled && item.mediaKind === "ANIMATED_IMAGE" ? () => beginAnimationHover(item.itemUUID) : undefined}
                      onAnimationHoverEnd={animationHoverEnabled && item.mediaKind === "ANIMATED_IMAGE" ? () => endAnimationHover(item.itemUUID) : undefined}
                      onFavorite={() => void toggleItemFavorite(item)}
                      onSetCover={() => void setItemAsCover(item)}
                    />
                  ))}
                </div>
                {visible[group.key] < group.items.length ? (
                  <button className="load-more" type="button" onClick={() => setVisible((current) => ({ ...current, [group.key]: current[group.key] + memberBatchSize }))}>
                    {intl.formatMessage({ id: "gallery.loadMore" })}
                  </button>
                ) : null}
              </section>
            ))}
          </div>
        </section>

        {relatedQuery.data?.relatedGalleries.length ? (
          <aside className="related-section">
            <h2>{intl.formatMessage({ id: "gallery.related" })}</h2>
            <div className="related-list">{relatedQuery.data.relatedGalleries.map(({ card }) => <RelatedGallery key={card.setID} card={card} />)}</div>
          </aside>
        ) : null}
      </div>

      {lightboxItem ? (
        <Lightbox
          key={lightboxItem.itemUUID}
          item={lightboxItem}
          favorite={favoriteOverrides[lightboxItem.itemUUID] ?? lightboxItem.favorite}
          rating={ratingOverrides[lightboxItem.itemUUID] ?? lightboxItem.ratingHalfSteps ?? null}
          currentCover={coverItemUUID === lightboxItem.itemUUID}
          personalControlsVisible={personalControlsVisible}
          busy={busyItemUUID === lightboxItem.itemUUID}
          onClose={closeLightbox}
          canPrevious={lightboxNavigation.canPrevious}
          canNext={lightboxNavigation.canNext}
          onPrevious={() => moveLightbox(lightboxIndex - 1)}
          onNext={() => moveLightbox(lightboxIndex + 1)}
          onFavorite={() => void toggleItemFavorite(lightboxItem)}
          onRating={(rating) => void rateItem(lightboxItem, rating)}
          onSetCover={() => void setItemAsCover(lightboxItem)}
        />
      ) : null}
    </main>
  );
}

function RelatedGallery({ card }: { card: BrowseGalleryCard }) {
  const presentation = galleryCardPresentation(card);
  const resourceURL = itemResourceURL(card.cover.resource);
  const mediaCount = formatGalleryMediaCount(card.media);
  return (
    <article className="related-gallery">
      <Link className="related-gallery__poster" to={`/gallery/${encodeURIComponent(card.slug)}`} aria-label={card.title}>
        {resourceURL ? <img src={resourceURL} alt="" loading="lazy" /> : <span>CGM</span>}
      </Link>
      <div>
        <h3><Link to={`/gallery/${encodeURIComponent(card.slug)}`}>{presentation.primary}</Link></h3>
        {presentation.secondary ? <p>{presentation.secondary}</p> : null}
        {mediaCount ? <span>{mediaCount}</span> : null}
      </div>
    </article>
  );
}

function MediaTile({ item, animate, favorite, currentCover, personalControlsVisible, busy, setElement, onOpen, onAnimationHoverStart, onAnimationHoverEnd, onFavorite, onSetCover }: {
  item: GalleryMember;
  animate: boolean;
  favorite: boolean;
  currentCover: boolean;
  personalControlsVisible: boolean;
  busy: boolean;
  setElement: (element: HTMLElement | null) => void;
  onOpen: () => void;
  onAnimationHoverStart?: () => void;
  onAnimationHoverEnd?: () => void;
  onFavorite: () => void;
  onSetCover: () => void;
}) {
  const intl = useIntl();
  const animatedPreview = useOnDemandAnimatedPreview(item, animate);
  const resource = animate && animatedPreview.resource ? animatedPreview.resource : item.cardResource;
  const menuRef = useRef<HTMLDetailsElement>(null);
  const [menuOpen, setMenuOpen] = useState(false);
  const closeMenu = () => {
    if (menuRef.current) menuRef.current.open = false;
  };
  useEffect(() => {
    if (!menuOpen) return;
    const dismissOnOutsidePointer = (event: PointerEvent) => {
      const menu = menuRef.current;
      if (menu && event.target instanceof Node && !menu.contains(event.target)) closeMenu();
    };
    const dismissOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") closeMenu();
    };
    document.addEventListener("pointerdown", dismissOnOutsidePointer);
    document.addEventListener("keydown", dismissOnEscape);
    return () => {
      document.removeEventListener("pointerdown", dismissOnOutsidePointer);
      document.removeEventListener("keydown", dismissOnEscape);
    };
  }, [menuOpen]);
  return (
    <article className="media-tile" ref={setElement} data-item-uuid={item.itemUUID} onPointerEnter={onAnimationHoverStart} onPointerLeave={onAnimationHoverEnd}>
      <button className="media-tile__open" type="button" onClick={onOpen} aria-label={item.caption || item.imageCategory || item.mediaKind}>
        {resource ? <img key={`${item.itemUUID}-${resource.variant}`} src={itemResourceURL(resource) ?? undefined} alt="" loading="lazy" /> : <span className="media-tile__pending">{item.processingState}</span>}
        {item.mediaKind !== "STATIC_IMAGE" ? <span className="media-tile__kind">{item.mediaKind === "VIDEO" ? "VIDEO" : "GIF"}</span> : null}
        {item.caption ? <span className="media-tile__caption">{item.caption}</span> : null}
      </button>
      {currentCover ? <span className="media-tile__cover"><Icon name="gallery" />{intl.formatMessage({ id: "gallery.currentCover" })}</span> : null}
      {personalControlsVisible ? <button className={`media-tile__favorite${favorite ? " is-active" : ""}`} type="button" disabled={busy} aria-label={intl.formatMessage({ id: favorite ? "gallery.itemUnfavorite" : "gallery.itemFavorite" })} aria-pressed={favorite} onClick={onFavorite}><Icon name="heart" /></button> : null}
      <details ref={menuRef} className="media-tile__menu" onToggle={(event) => setMenuOpen(event.currentTarget.open)}>
        <summary aria-label={intl.formatMessage({ id: "gallery.itemMenu" })}><Icon name="more-horizontal" /></summary>
        <div>
          <Link to={`/media/${item.itemUUID}`} onClick={closeMenu}><Icon name="info" />{intl.formatMessage({ id: "gallery.openMedia" })}</Link>
          {item.mediaKind === "STATIC_IMAGE" ? <button type="button" disabled={busy || currentCover} onClick={() => { closeMenu(); onSetCover(); }}><Icon name="gallery" />{intl.formatMessage({ id: currentCover ? "gallery.currentCover" : "gallery.setCover" })}</button> : null}
        </div>
      </details>
    </article>
  );
}

function Lightbox({ item, favorite, rating, currentCover, personalControlsVisible, busy, onClose, canPrevious, canNext, onPrevious, onNext, onFavorite, onRating, onSetCover }: {
  item: GalleryMember;
  favorite: boolean;
  rating: number | null;
  currentCover: boolean;
  personalControlsVisible: boolean;
  busy: boolean;
  onClose: () => void;
  canPrevious: boolean;
  canNext: boolean;
  onPrevious: () => void;
  onNext: () => void;
  onFavorite: () => void;
  onRating: (rating: number | null) => void;
  onSetCover: () => void;
}) {
  const intl = useIntl();
  const imageDisplay = useImageDisplay(item);
  const videoPlayback = useOnDemandVideoPlayback(item);
  const resourceURL = item.mediaKind === "VIDEO" ? videoPlayback.url : imageDisplay.url;
  const closeRef = useRef<HTMLButtonElement>(null);
  const favoriteLabel = intl.formatMessage({ id: favorite ? "gallery.itemUnfavorite" : "gallery.itemFavorite" });
  const ratingLabel = intl.formatMessage({ id: "gallery.rating" });
  const coverLabel = intl.formatMessage({ id: currentCover ? "gallery.currentCover" : "gallery.setCover" });
  const mediaDetailsLabel = intl.formatMessage({ id: "gallery.openMedia" });
  const closeLabel = intl.formatMessage({ id: "gallery.closeViewer" });
  useEffect(() => {
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    closeRef.current?.focus();
    return () => { document.body.style.overflow = previousOverflow; };
  }, []);
  return (
    <div className="lightbox" role="dialog" aria-modal="true" aria-label={item.caption || item.mediaKind} onClick={onClose}
      onKeyDown={(event) => { if (event.key === "Escape") onClose(); else if (event.key === "ArrowLeft" && canPrevious) onPrevious(); else if (event.key === "ArrowRight" && canNext) onNext(); }}>
      <div className="lightbox__toolbar" onClick={(event) => event.stopPropagation()}>
        {personalControlsVisible ? <>
          <button className={favorite ? "is-active" : ""} type="button" disabled={busy} aria-label={favoriteLabel} title={favoriteLabel} aria-pressed={favorite} onClick={onFavorite}><Icon name="heart" /></button>
          <select aria-label={ratingLabel} title={ratingLabel} value={rating ?? 0} disabled={busy} onChange={(event) => onRating(Number(event.target.value) || null)}>
            <option value={0}>—</option>{Array.from({ length: 10 }, (_, index) => index + 1).map((value) => <option key={value} value={value}>{value / 2} / 5</option>)}
          </select>
        </> : null}
        {item.mediaKind === "STATIC_IMAGE" ? <button type="button" disabled={busy || currentCover} aria-label={coverLabel} title={coverLabel} onClick={onSetCover}><Icon name="gallery" /></button> : null}
        <Link to={`/media/${item.itemUUID}`} aria-label={mediaDetailsLabel} title={mediaDetailsLabel}><Icon name="info" /></Link>
        <button ref={closeRef} className="lightbox__close" type="button" aria-label={closeLabel} title={closeLabel} onClick={onClose}><Icon name="close" /></button>
      </div>
      <button className="lightbox__previous" type="button" aria-label="Previous media" disabled={!canPrevious} onClick={(event) => { event.stopPropagation(); onPrevious(); }}><Icon name="chevron-left" /></button>
      <div className="lightbox__content" onClick={(event) => event.stopPropagation()}>
        {item.mediaKind === "VIDEO" && resourceURL ? <video key={item.itemUUID} src={resourceURL} controls playsInline autoPlay />
          : item.mediaKind !== "VIDEO" && resourceURL ? <img key={item.itemUUID} src={resourceURL} alt={item.caption} />
            : item.cardResource ? <img key={`${item.itemUUID}-poster`} src={itemResourceURL(item.cardResource) ?? undefined} alt={item.caption} />
              : <span>{item.processingState}</span>}
        {item.mediaKind === "VIDEO" && videoPlayback.preparing ? <span className="lightbox__status">Preparing compatible video…</span> : null}
        {item.mediaKind === "VIDEO" && videoPlayback.failed ? <span className="lightbox__status" role="alert">Video is temporarily unavailable{videoPlayback.errorCode ? ` (${videoPlayback.errorCode})` : ""}. <button type="button" onClick={videoPlayback.retry}>Retry</button></span> : null}
        {item.mediaKind !== "VIDEO" && imageDisplay.preparing ? <span className="lightbox__status">Preparing full-size view…</span> : null}
        {item.mediaKind !== "VIDEO" && imageDisplay.failed ? <span className="lightbox__status" role="alert">Full-size view is temporarily unavailable.</span> : null}
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
