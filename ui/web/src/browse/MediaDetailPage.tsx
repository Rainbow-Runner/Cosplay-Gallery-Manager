import { useMutation, useQuery } from "@apollo/client/react";
import { useEffect, useState } from "react";
import { useIntl } from "react-intl";
import { Link, useParams } from "react-router-dom";
import { BROWSE_UI_SETTINGS, MEDIA_DETAIL, MEDIA_EMBEDDED_METADATA, RECORD_GALLERY_VIEW, SET_ITEM_FAVORITE, SET_ITEM_RATING } from "../api/browse";
import { readMediaMetadataVisibleFields } from "./mediaMetadataPreferences";
import { itemResourceURL } from "./resourceUrl";
import type { BrowseUISettings, MediaDetail, MediaInformation } from "./types";
import { useImageDisplay } from "./useImageDisplay";
import { useOnDemandVideoPlayback } from "./useOnDemandVideoPlayback";

export function MediaDetailPage() {
  const intl = useIntl(); const { uuid = "" } = useParams(); const query = useQuery<{ mediaDetail: MediaDetail }>(MEDIA_DETAIL, { variables: { itemUUID: uuid }, fetchPolicy: "cache-and-network" });
  const [recordView] = useMutation(RECORD_GALLERY_VIEW);
  const [setFavorite] = useMutation(SET_ITEM_FAVORITE); const [setRating] = useMutation(SET_ITEM_RATING);
  const settings = useQuery<{ browseUISettings: BrowseUISettings }>(BROWSE_UI_SETTINGS);
  const [visibleMetadataFields] = useState(readMediaMetadataVisibleFields);
  const metadataQuery = useQuery<{ mediaEmbeddedMetadata: MediaInformation }>(MEDIA_EMBEDDED_METADATA, { variables: { itemUUID: uuid, visibleFields: visibleMetadataFields }, skip: !query.data, fetchPolicy: "network-only" });
  const [favorite, updateFavorite] = useState(false); const [rating, updateRating] = useState<number | null>(null); const [revision, updateRevision] = useState(0);
  useEffect(() => { if (query.data) void recordView({ variables: { setID: query.data.mediaDetail.gallery.setID, itemUUID: uuid } }); }, [query.data, recordView, uuid]);
  useEffect(() => { if (query.data) { updateFavorite(query.data.mediaDetail.item.favorite); updateRating(query.data.mediaDetail.item.ratingHalfSteps ?? null); updateRevision(query.data.mediaDetail.metadataRevision); } }, [query.data]);
	const imageDisplay = useImageDisplay(query.data?.mediaDetail.item);
  const videoPlayback = useOnDemandVideoPlayback(query.data?.mediaDetail.item);
  if (query.loading) return <main className="browse-main"><p className="state-message">{intl.formatMessage({ id: "state.loading" })}</p></main>;
  if (query.error || !query.data) return <main className="browse-main"><p className="state-message" role="alert">{intl.formatMessage({ id: "state.error" })}</p></main>;
	const detail = query.data.mediaDetail; const resource = detail.displayResource ?? detail.item.cardResource;
  const resourceURL = detail.item.mediaKind === "VIDEO" ? itemResourceURL(resource) : imageDisplay.url ?? itemResourceURL(resource);
  const mediaInformation = metadataQuery.data?.mediaEmbeddedMetadata;
	const informationGroups = groupMediaInformation(mediaInformation?.entries ?? []);
  return <main className="media-detail"><section className="media-detail__stage">{detail.item.mediaKind === "VIDEO" && videoPlayback.url
    ? <video key={detail.item.itemUUID} src={videoPlayback.url} controls playsInline /> : resourceURL ? <img src={resourceURL} alt={detail.item.caption} />
      : <span>{detail.item.processingState}</span>}{detail.item.mediaKind === "VIDEO" && videoPlayback.preparing ? <span className="media-detail__status">Preparing compatible video…</span> : null}{detail.item.mediaKind === "VIDEO" && videoPlayback.failed ? <span className="media-detail__status" role="alert">Video is temporarily unavailable{videoPlayback.errorCode ? ` (${videoPlayback.errorCode})` : ""}. <button type="button" onClick={videoPlayback.retry}>Retry</button></span> : null}{detail.item.mediaKind !== "VIDEO" && imageDisplay.preparing ? <span className="media-detail__status">Preparing full-size view…</span> : null}{detail.item.mediaKind !== "VIDEO" && imageDisplay.failed ? <span className="media-detail__status" role="alert">Full-size view is temporarily unavailable.</span> : null}{detail.item.caption ? <p>{detail.item.caption}</p> : null}</section>
    <aside className="media-detail__sidebar"><p>{detail.gallery.collectionType}</p><h1>{detail.gallery.title}</h1>
      <div className="media-detail__relations"><span>{detail.gallery.characters.map((value) => value.name).join(" · ") || "\u00a0"}</span><span>{detail.gallery.credits.map((value) => value.name).join(" · ")}</span></div>
      <section className="media-detail__information" aria-labelledby="media-information-heading"><h2 id="media-information-heading">{intl.formatMessage({ id: "media.information" })}</h2>
        {metadataQuery.loading ? <p>{intl.formatMessage({ id: "media.metadataPending" })}</p> : null}
        {metadataQuery.error ? <p role="status">{intl.formatMessage({ id: "media.metadataError" })}</p> : null}
        {mediaInformation?.state === "ERROR" ? <p role="status">{intl.formatMessage({ id: "media.metadataError" })}{mediaInformation.errorCode ? ` (${mediaInformation.errorCode})` : ""}</p> : null}
        {informationGroups.map(([group, entries]) => <div className="media-detail__information-group" key={group}><h3>{intl.formatMessage({ id: `media.metadataGroup.${group.toLowerCase()}` })}</h3><dl>{entries.map((entry) => <div key={entry.key}><dt>{metadataLabel(entry.visibilityKey, entry.label, intl)}</dt><dd>{entry.value}</dd></div>)}</dl></div>)}
        {!informationGroups.length && mediaInformation?.state === "READY" ? <p>{intl.formatMessage({ id: "media.metadataEmpty" })}</p> : null}
      </section>
      {settings.data?.browseUISettings.detailRatingControlVisible ? <div className="media-detail__personal"><button type="button" onClick={async () => { const next = !favorite; updateFavorite(next); try { await setFavorite({ variables: { itemUUID: uuid, favorite: next } }); } catch { updateFavorite(!next); } }}>{favorite ? "♥" : "♡"}</button>
        <select aria-label="Rating" value={rating ?? 0} onChange={async (event) => { const next = Number(event.target.value) || null; const previous = rating; updateRating(next); try { await setRating({ variables: { itemUUID: uuid, ratingHalfSteps: next, expectedMetadataRevision: revision } }); updateRevision((current) => current + 1); } catch { updateRating(previous); } }}>
          <option value={0}>—</option>{Array.from({ length: 10 }, (_, index) => index + 1).map((value) => <option key={value} value={value}>{value / 2} / 5</option>)}</select></div> : null}
      <Link to={`/gallery/${detail.gallery.slug}`}>{intl.formatMessage({ id: "media.gallery" })}</Link></aside></main>;
}

function groupMediaInformation(entries: MediaInformation["entries"]) {
  const groups = new Map<string, typeof entries>();
  for (const entry of entries) groups.set(entry.group, [...(groups.get(entry.group) ?? []), entry]);
  return [...groups.entries()];
}

function metadataLabel(visibilityKey: string, fallback: string, intl: ReturnType<typeof useIntl>) {
  const id = metadataLabelMessages[visibilityKey];
  return id ? intl.formatMessage({ id }) : fallback;
}

const metadataLabelMessages: Record<string, string> = {
  "file.type": "media.metadata.type", "file.size": "media.metadata.size", "file.dimensions": "media.metadata.dimensions", "file.added_at": "media.metadata.addedAt",
  "descriptive.title": "media.metadata.title", "descriptive.description": "media.metadata.description", "descriptive.author": "media.metadata.author", "descriptive.source": "media.metadata.source", "descriptive.software": "media.metadata.software", "descriptive.copyright": "media.metadata.copyright", "descriptive.keywords": "media.metadata.keywords", "descriptive.comment": "media.metadata.comment",
  "date.modified": "media.metadata.modified", "date.original": "media.metadata.taken", "date.digitized": "media.metadata.digitized",
  "camera.make": "media.metadata.cameraMake", "camera.model": "media.metadata.cameraModel", "camera.lens": "media.metadata.lens", "camera.exposure_time": "media.metadata.exposureTime", "camera.aperture": "media.metadata.aperture", "camera.iso": "media.metadata.iso", "camera.focal_length": "media.metadata.focalLength", "camera.exposure_bias": "media.metadata.exposureBias", "camera.flash": "media.metadata.flash", "camera.metering_mode": "media.metadata.metering", "camera.white_balance": "media.metadata.whiteBalance",
  "image.orientation": "media.metadata.orientation", "image.color_space": "media.metadata.colorSpace", "image.resolution": "media.metadata.resolution",
  "video.duration": "media.metadata.duration", "video.container": "media.metadata.container", "video.codec": "media.metadata.videoCodec", "video.frame_rate": "media.metadata.frameRate", "video.audio_codec": "media.metadata.audioCodec",
  "sensitive.gps": "media.metadata.gps", "sensitive.device_identifiers": "media.metadata.identifier",
};
