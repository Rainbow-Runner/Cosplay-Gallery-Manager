import { useQuery } from "@apollo/client/react";
import { useIntl } from "react-intl";
import { useSearchParams } from "react-router-dom";
import { TIMELINE_GALLERIES } from "../api/browse";
import { GalleryCard } from "./GalleryCard";
import { ScopeSelector } from "./ScopeSelector";
import type { GalleryPage, Scope } from "./types";

export function TimelinePage({ coserUUID }: { coserUUID?: string }) {
  const intl = useIntl(); const [parameters, setParameters] = useSearchParams();
  const scope = (parameters.get("scope") as Scope) || "LIST"; const page = Math.max(1, Number(parameters.get("page")) || 1);
  const { data, loading, error } = useQuery<{ timelineGalleries: GalleryPage }>(TIMELINE_GALLERIES,
    { variables: { scope, page, coserUUID: coserUUID || null } });
  return <main className="browse-main"><header className="page-heading"><p>{scope}</p><h1>{intl.formatMessage({ id: "page.timeline" })}</h1></header>
    <ScopeSelector value={scope} onChange={(next) => setParameters({ scope: next, page: "1" })} />
    {loading ? <p className="state-message">{intl.formatMessage({ id: "state.loading" })}</p> : null}
    {error ? <p className="state-message" role="alert">{intl.formatMessage({ id: "state.error" })}</p> : null}
    {data?.timelineGalleries.items.length ? <section className="gallery-grid">{data.timelineGalleries.items.map((card) => <GalleryCard key={card.setID} card={card} scrubberEnabled={false} />)}</section>
      : !loading && !error ? <p className="state-message">{intl.formatMessage({ id: "state.empty" })}</p> : null}
  </main>;
}
