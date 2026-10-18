import { useQuery } from "@apollo/client/react";
import { useIntl } from "react-intl";
import { useSearchParams } from "react-router-dom";
import { TIMELINE_GALLERIES } from "../api/browse";
import { GalleryCard } from "./GalleryCard";
import { CoserMonthTimeline } from "./CoserMonthTimeline";
import { ScopeSelector } from "./ScopeSelector";
import type { GalleryPage, Scope } from "./types";

export function TimelinePage({ coserUUID }: { coserUUID?: string }) {
  const intl = useIntl(); const [parameters, setParameters] = useSearchParams();
  const scope = (parameters.get("scope") as Scope) || (coserUUID ? "ALL" : "LIST"); const page = Math.max(1, Number(parameters.get("page")) || 1);
  const requestedDate = parameters.get("date");
  const date: "SHOOT" | "MEDIA_ADDED" | "PUBLISH" | "COMBINED" = coserUUID && (requestedDate === "MEDIA_ADDED" || requestedDate === "PUBLISH" || requestedDate === "COMBINED") ? requestedDate : "SHOOT";
  const { data, loading, error } = useQuery<{ timelineGalleries: GalleryPage }>(TIMELINE_GALLERIES,
    { variables: { scope, page, coserUUID: coserUUID || null, date }, skip: Boolean(coserUUID) });
  return <main className="browse-main"><header className="page-heading"><p>{scope}</p><h1>{intl.formatMessage({ id: "page.timeline" })}</h1></header>
    <ScopeSelector value={scope} onChange={(next) => setParameters({ scope: next, date, page: "1" })} />
    {coserUUID ? <label className="timeline-date-selector">{intl.formatMessage({ id: "timeline.dateSource" })}<select aria-label={intl.formatMessage({ id: "timeline.dateSource" })} value={date} onChange={(event) => setParameters({ scope, date: event.target.value, page: "1" })}><option value="SHOOT">{intl.formatMessage({ id: "timeline.shoot" })}</option><option value="MEDIA_ADDED">{intl.formatMessage({ id: "timeline.added" })}</option><option value="PUBLISH">{intl.formatMessage({ id: "timeline.publish" })}</option><option value="COMBINED">{intl.formatMessage({ id: "timeline.combined" })}</option></select></label> : null}
    {coserUUID && date === "COMBINED" ? <p className="timeline-date-help">{intl.formatMessage({ id: "timeline.combinedHelp" })}</p> : null}
    {coserUUID ? <CoserMonthTimeline key={`${coserUUID}:${scope}:${date}`} coserUUID={coserUUID} scope={scope} date={date} /> : <>
    {loading ? <p className="state-message">{intl.formatMessage({ id: "state.loading" })}</p> : null}
    {error ? <p className="state-message" role="alert">{intl.formatMessage({ id: "state.error" })}</p> : null}
    {data?.timelineGalleries.items.length ? <section className="gallery-grid">{data.timelineGalleries.items.map((card) => <GalleryCard key={card.setID} card={card} scrubberEnabled={false} />)}</section>
      : !loading && !error ? <p className="state-message">{intl.formatMessage({ id: "state.empty" })}</p> : null}
    {data?.timelineGalleries.totalPages && data.timelineGalleries.totalPages > 1 ? <nav className="browse-pagination" aria-label={intl.formatMessage({ id: "timeline.pagination" })}><button type="button" disabled={page <= 1} onClick={() => setParameters({ scope, date, page: String(page - 1) })}>‹</button><span>{page} / {data.timelineGalleries.totalPages}</span><button type="button" disabled={page >= data.timelineGalleries.totalPages} onClick={() => setParameters({ scope, date, page: String(page + 1) })}>›</button></nav> : null}
    </>}
  </main>;
}
