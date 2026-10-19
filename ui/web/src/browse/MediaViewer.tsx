import { useQuery } from "@apollo/client/react";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { useIntl } from "react-intl";
import { createPortal } from "react-dom";
import { Link } from "react-router-dom";
import { MEDIA_EMBEDDED_METADATA } from "../api/browse";
import { Icon } from "../ui/Icon";
import { MediaInformationPanel } from "./MediaInformationPanel";
import { readMediaMetadataVisibleFields } from "./mediaMetadataPreferences";
import type { GalleryMember, MediaInformation } from "./types";
import { itemResourceURL } from "./resourceUrl";
import { useImageDisplay } from "./useImageDisplay";
import { useOnDemandVideoPlayback } from "./useOnDemandVideoPlayback";
import { PlaybackVideo } from "./PlaybackVideo";
import { VideoPlaybackNotice } from "./VideoPlaybackNotice";
import { useViewerIsolation } from "./useViewerIsolation";

export function MediaViewer({ item, informationOpen, onInformationChange, onClose, informationAllowed = true, galleryHref, actions, canPrevious = false, canNext = false, onPrevious, onNext }: {
  item: GalleryMember; informationOpen: boolean; onInformationChange: (value: boolean) => void; onClose: () => void;
  informationAllowed?: boolean; galleryHref?: string; actions?: ReactNode;
  canPrevious?: boolean; canNext?: boolean; onPrevious?: () => void; onNext?: () => void;
}) {
  const intl = useIntl(); const t = (key: string) => intl.formatMessage({ id: key });
  const panelOpen = informationAllowed && informationOpen;
  const [visibleFields] = useState(() => readMediaMetadataVisibleFields().filter(key => key !== "file.added_at"));
  const metadata = useQuery<{ mediaEmbeddedMetadata: MediaInformation }>(MEDIA_EMBEDDED_METADATA, {
    variables: { itemUUID: item.itemUUID, visibleFields }, skip: !panelOpen || !visibleFields.length, fetchPolicy: "network-only",
  });
  // Apollo discards data from superseded variables. Never use previousData
  // here: the previous image's properties must not appear next to a new image.
  const currentMetadata: MediaInformation | undefined = !visibleFields.length ? {state:"READY",errorCode:"",entries:[]} : metadata.variables?.itemUUID === item.itemUUID ? metadata.data?.mediaEmbeddedMetadata : undefined;
  const image = useImageDisplay(item); const video = useOnDemandVideoPlayback(item);
  const url = item.mediaKind === "VIDEO" ? video.url : image.url ?? itemResourceURL(item.cardResource);
  const root = useRef<HTMLDivElement>(null); const close = useRef<HTMLButtonElement>(null); const information = useRef<HTMLButtonElement>(null);
  const scroll = useRef<HTMLDivElement>(null);
  const exit = useRef(onClose); exit.current = onClose;
  useViewerIsolation(close);
  useEffect(() => { if (scroll.current) scroll.current.scrollTop = 0; }, [item.itemUUID]);
  const title = currentMetadata?.entries.find(entry => entry.visibilityKey === "file.name")?.value || item.caption || t("media.viewer.title");
  const toggleLabel = t(panelOpen ? "media.viewer.hideInformation" : "gallery.openMedia");
  return createPortal(<div ref={root} className={`media-viewer lightbox${panelOpen ? " media-viewer--information" : ""}`} role="dialog" aria-modal="true" aria-label={item.caption || item.mediaKind}
    onKeyDown={event => {
      if (event.key === "Escape") { event.preventDefault(); event.stopPropagation(); if (panelOpen) { onInformationChange(false); information.current?.focus(); } else exit.current(); return; }
      const target = event.target as HTMLElement;
      const editing = target.matches("input,textarea,select") || target.isContentEditable || !!target.closest(".media-viewer__information");
      if (!editing && event.key === "ArrowLeft" && canPrevious) { event.preventDefault(); onPrevious?.(); }
      if (!editing && event.key === "ArrowRight" && canNext) { event.preventDefault(); onNext?.(); }
      if (event.key === "Tab") {
        const controls = [...(root.current?.querySelectorAll<HTMLElement>('button:not([disabled]),a[href],input,select,summary,[tabindex="0"]') ?? [])].filter(element => element.getClientRects().length > 0);
        const first = controls[0], last = controls.at(-1);
        if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus(); }
        else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus(); }
      }
    }}>
    <header className="media-viewer__toolbar"><span className="media-viewer__filename" title={title}>{title}</span><div className="media-viewer__actions">{actions}
      {galleryHref ? <Link to={galleryHref} className="media-viewer__gallery" title={t("media.gallery")} aria-label={t("media.gallery")}><Icon name="gallery" /><span>{t("media.gallery")}</span></Link> : null}
      {informationAllowed ? <button ref={information} type="button" aria-expanded={panelOpen} aria-controls="media-viewer-information" aria-label={toggleLabel} title={toggleLabel} onClick={() => onInformationChange(!panelOpen)}><Icon name="info" /></button> : null}
      <button ref={close} className="lightbox__close" type="button" onClick={onClose} aria-label={t("gallery.closeViewer")} title={t("gallery.closeViewer")}><Icon name="close" /></button>
    </div></header>
    <div className="media-viewer__body">
      <section className="media-viewer__stage" aria-label={t("media.viewer.image")}>
        <div className="media-viewer__image lightbox__content">{item.mediaKind === "VIDEO" && url ? <PlaybackVideo key={item.itemUUID} url={url} hls={video.hls} autoPlay onError={video.onPlaybackError} /> : url ? <img key={item.itemUUID} src={url} alt={item.caption} /> : <p role="status">{t("state.loading")}</p>}</div>
        {onPrevious ? <button className="media-viewer__previous lightbox__previous" type="button" aria-label={t("media.viewer.previous")} disabled={!canPrevious} onClick={onPrevious}><Icon name="chevron-left" /></button> : null}
        {onNext ? <button className="media-viewer__next lightbox__next" type="button" aria-label={t("media.viewer.next")} disabled={!canNext} onClick={onNext}><Icon name="chevron-right" /></button> : null}
        {item.mediaKind === "VIDEO" ? <VideoPlaybackNotice state={video} className="media-viewer__status" /> : image.preparing ? <p className="media-viewer__status" role="status">{t("state.loading")}</p> : image.failed ? <p className="media-viewer__status" role="status">{t("media.metadataError")}</p> : null}
      </section>
      {panelOpen ? <aside id="media-viewer-information" className="media-viewer__information" aria-label={t("media.information")}><header><h2>{t("media.information")}</h2><button type="button" aria-label={t("media.viewer.hideInformation")} onClick={() => { onInformationChange(false); information.current?.focus(); }}><Icon name="close" /></button></header>
        <div ref={scroll} className="media-viewer__information-scroll"><MediaInformationPanel information={currentMetadata} loading={metadata.loading} error={!!metadata.error} historyEnabled={item.mediaKind !== "VIDEO" && visibleFields.some(key => key.startsWith("history."))} /></div>
      </aside> : null}
    </div>
  </div>, document.body);
}
