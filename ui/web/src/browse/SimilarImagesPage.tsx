import { useLazyQuery } from "@apollo/client/react";
import { useState, type FormEvent } from "react";
import { useIntl } from "react-intl";
import { Link } from "react-router-dom";
import { SIMILAR_IMAGES } from "../api/browse";
import { itemResourceURL } from "./resourceUrl";
import type { ResourceIdentity } from "./types";

export interface SimilarImage {
  itemUUID: string; gallerySetID: string; gallerySlug: string; galleryTitle: string; fileName: string; resource: ResourceIdentity;
}
export interface SimilarImageResult {
  totalImages: number; hashedImages: number; comparedImages: number; comparisons: number; matchedPairs: number;
  complete: boolean; hasMore: boolean;
  pairs: { left: SimilarImage; right: SimilarImage; pHashDistance: number; dHashDistance: number; sameContent: boolean }[];
}
type MatchScope = "ALL" | "SAME_GALLERY" | "CROSS_GALLERY";

export function SimilarImagesPage() {
  const intl = useIntl();
  const text = (key: string) => intl.formatMessage({ id: `similar.${key}` });
  const [maxDistance, setMaxDistance] = useState(4);
  const [scope, setScope] = useState<MatchScope>("ALL");
  const [applied, setApplied] = useState({ maxDistance: 4, scope: "ALL" as MatchScope });
  const [visible, setVisible] = useState(24);
  const [search, query] = useLazyQuery<{ similarImages: SimilarImageResult }>(SIMILAR_IMAGES, { fetchPolicy: "no-cache", notifyOnNetworkStatusChange: true });
  function submit(event: FormEvent) {
    event.preventDefault(); if (query.loading) return;
    const variables = { maxDistance, scope }; setApplied(variables); setVisible(24);
    void search({ variables }).catch(() => { /* Show the query error, never a successful empty result. */ });
  }
  const result = !query.loading && !query.error ? query.data?.similarImages : undefined;
  return <main className="browse-main similar-images-page">
    <header className="page-heading"><h1>{text("title")}</h1></header>
    <p className="similar-images-help">{text("help")}</p>
    <form className="similar-images-form" onSubmit={submit}>
      <label>{text("distance")}<select disabled={query.loading} value={maxDistance} onChange={(event) => setMaxDistance(Number(event.target.value))}>
        {[0, 2, 4, 6].map((value) => <option key={value} value={value}>{value}{value === 0 ? ` · ${text("zero")}` : ""}</option>)}
      </select></label>
      <label>{text("scope")}<select disabled={query.loading} value={scope} onChange={(event) => setScope(event.target.value as MatchScope)}>
        {(["ALL", "SAME_GALLERY", "CROSS_GALLERY"] as const).map((value) => <option key={value} value={value}>{text(value)}</option>)}
      </select></label>
      <button className="load-more" disabled={query.loading} type="submit">{text(query.loading ? "searching" : "search")}</button>
    </form>
    <details className="similar-images-rules"><summary>{text("rules")}</summary><p className="similar-images-help">{text("limits")}</p></details>
    {query.loading ? <p role="status" className="state-message">{text("searching")}</p> : null}
    {query.error ? <p role="alert" className="state-message">{text("error")}</p> : null}
    {!query.called ? <p className="state-message">{text("idle")}</p> : null}
    {result ? <section aria-label={text("results")}>
      <div className="similar-images-summary" role="status">
        <p>{intl.formatMessage({ id: "similar.summary" }, { total: result.totalImages, hashed: result.hashedImages, compared: result.comparedImages, matches: result.matchedPairs })}</p>
        <p>pHash ≤ {applied.maxDistance} · {text(applied.scope)}</p>
        {result.hashedImages < result.totalImages ? <p>{text("unhashed")}</p> : null}
        {!result.complete ? <p role="alert">{text("incomplete")}</p> : null}
        {result.hasMore ? <p>{text("capped")}</p> : null}
      </div>
      {!result.pairs.length ? <p className="state-message">{text(result.complete ? "empty" : "partialEmpty")}</p> : <div className="similar-images-results">
        {result.pairs.slice(0, visible).map((pair) => <article className="similar-images-pair" key={`${pair.left.itemUUID}-${pair.right.itemUUID}`}>
          <header><strong>pHash {pair.pHashDistance} / 64 · dHash {pair.dHashDistance} / 128</strong><span>{text(pair.sameContent ? "sameContent" : "review")}</span></header>
          <div className="similar-images-pair__images">{[pair.left, pair.right].map((image) => <div key={image.itemUUID}>
            <Link className="similar-images-preview" to={`/media/${encodeURIComponent(image.itemUUID)}`} aria-label={`${text("open")} · ${image.fileName}`}><img src={itemResourceURL(image.resource) ?? undefined} alt={image.fileName} loading="lazy" decoding="async" /></Link>
            <p title={image.fileName}>{image.fileName}</p><Link title={image.galleryTitle} to={`/gallery/${encodeURIComponent(image.gallerySlug)}`}>{image.galleryTitle}</Link>
          </div>)}</div>
        </article>)}
      </div>}
      {visible < result.pairs.length ? <button type="button" className="load-more" onClick={() => setVisible((current) => current + 24)}>{text("more")}</button> : null}
    </section> : null}
  </main>;
}
