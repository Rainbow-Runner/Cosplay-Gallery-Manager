import { useMutation, useQuery } from "@apollo/client/react";
import { useEffect, useState } from "react";
import { useIntl } from "react-intl";
import { Link, useParams } from "react-router-dom";
import { BROWSE_UI_SETTINGS, MEDIA_DETAIL, RECORD_GALLERY_VIEW, SET_ITEM_FAVORITE, SET_ITEM_RATING } from "../api/browse";
import { itemResourceURL } from "./resourceUrl";
import type { BrowseUISettings, MediaDetail } from "./types";
import { useOnDemandLightbox } from "./useOnDemandLightbox";
import { useOnDemandVideoPlayback } from "./useOnDemandVideoPlayback";

export function MediaDetailPage() {
  const intl = useIntl(); const { uuid = "" } = useParams(); const query = useQuery<{ mediaDetail: MediaDetail }>(MEDIA_DETAIL, { variables: { itemUUID: uuid } });
  const [recordView] = useMutation(RECORD_GALLERY_VIEW);
  const [setFavorite] = useMutation(SET_ITEM_FAVORITE); const [setRating] = useMutation(SET_ITEM_RATING);
  const settings = useQuery<{ browseUISettings: BrowseUISettings }>(BROWSE_UI_SETTINGS);
  const [favorite, updateFavorite] = useState(false); const [rating, updateRating] = useState<number | null>(null); const [revision, updateRevision] = useState(0);
  useEffect(() => { if (query.data) void recordView({ variables: { setID: query.data.mediaDetail.gallery.setID, itemUUID: uuid } }); }, [query.data, recordView, uuid]);
  useEffect(() => { if (query.data) { updateFavorite(query.data.mediaDetail.item.favorite); updateRating(query.data.mediaDetail.item.ratingHalfSteps ?? null); updateRevision(query.data.mediaDetail.metadataRevision); } }, [query.data]);
	const onDemand = useOnDemandLightbox(query.data?.mediaDetail.item);
  const videoPlayback = useOnDemandVideoPlayback(query.data?.mediaDetail.item);
  if (query.loading) return <main className="browse-main"><p className="state-message">{intl.formatMessage({ id: "state.loading" })}</p></main>;
  if (query.error || !query.data) return <main className="browse-main"><p className="state-message" role="alert">{intl.formatMessage({ id: "state.error" })}</p></main>;
	const detail = query.data.mediaDetail; const resource = detail.item.mediaKind === "STATIC_IMAGE"
		? onDemand.resource ?? detail.item.largeResource ?? detail.item.cardResource
		: detail.displayResource ?? detail.item.cardResource;
  const resourceURL = itemResourceURL(resource);
  return <main className="media-detail"><section className="media-detail__stage">{detail.item.mediaKind === "VIDEO" && videoPlayback.url
    ? <video key={detail.item.itemUUID} src={videoPlayback.url} controls playsInline /> : resourceURL ? <img src={resourceURL} alt={detail.item.caption} />
      : <span>{detail.item.processingState}</span>}{detail.item.mediaKind === "VIDEO" && videoPlayback.preparing ? <span className="media-detail__status">Preparing compatible video…</span> : null}{detail.item.mediaKind === "VIDEO" && videoPlayback.failed ? <span className="media-detail__status" role="alert">Video is temporarily unavailable{videoPlayback.errorCode ? ` (${videoPlayback.errorCode})` : ""}. <button type="button" onClick={videoPlayback.retry}>Retry</button></span> : null}{detail.item.mediaKind !== "VIDEO" && onDemand.preparing ? <span className="media-detail__status">Preparing full-size view…</span> : null}{detail.item.mediaKind !== "VIDEO" && onDemand.failed ? <span className="media-detail__status" role="alert">Full-size view is temporarily unavailable.</span> : null}{detail.item.caption ? <p>{detail.item.caption}</p> : null}</section>
    <aside className="media-detail__sidebar"><p>{detail.gallery.collectionType}</p><h1>{detail.gallery.title}</h1>
      <div className="media-detail__relations"><span>{detail.gallery.characters.map((value) => value.name).join(" · ") || "\u00a0"}</span><span>{detail.gallery.credits.map((value) => value.name).join(" · ")}</span></div>
      {detail.videoTechnical ? <dl className="media-detail__technical"><dt>Duration</dt><dd>{formatDuration(detail.videoTechnical.durationSeconds)}</dd><dt>Dimensions</dt><dd>{detail.videoTechnical.width} × {detail.videoTechnical.height}</dd><dt>Container</dt><dd>{detail.videoTechnical.container || "Unknown"}</dd><dt>Video</dt><dd>{detail.videoTechnical.videoCodec || "Unknown"}{detail.videoTechnical.frameRate ? ` · ${detail.videoTechnical.frameRate.toFixed(2)} fps` : ""}</dd><dt>Audio</dt><dd>{detail.videoTechnical.audioCodec || "None"}</dd></dl> : null}
      {settings.data?.browseUISettings.detailRatingControlVisible ? <div className="media-detail__personal"><button type="button" onClick={async () => { const next = !favorite; updateFavorite(next); try { await setFavorite({ variables: { itemUUID: uuid, favorite: next } }); } catch { updateFavorite(!next); } }}>{favorite ? "♥" : "♡"}</button>
        <select aria-label="Rating" value={rating ?? 0} onChange={async (event) => { const next = Number(event.target.value) || null; const previous = rating; updateRating(next); try { await setRating({ variables: { itemUUID: uuid, ratingHalfSteps: next, expectedMetadataRevision: revision } }); updateRevision((current) => current + 1); } catch { updateRating(previous); } }}>
          <option value={0}>—</option>{Array.from({ length: 10 }, (_, index) => index + 1).map((value) => <option key={value} value={value}>{value / 2} / 5</option>)}</select></div> : null}
      <Link to={`/gallery/${detail.gallery.slug}`}>{intl.formatMessage({ id: "media.gallery" })}</Link></aside></main>;
}

function formatDuration(value: number) { const seconds=Math.max(0,Math.round(value)); const minutes=Math.floor(seconds/60); return `${minutes}:${String(seconds%60).padStart(2,"0")}`; }
