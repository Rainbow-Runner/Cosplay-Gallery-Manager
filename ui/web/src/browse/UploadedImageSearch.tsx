import { useEffect, useRef, useState, type FormEvent } from "react";
import { useIntl } from "react-intl";
import { Link, useLocation } from "react-router-dom";
import { itemResourceURL } from "./resourceUrl";
import type { SimilarImage } from "./SimilarImagesPage";

export interface UploadedImageResult {
  totalImages: number; hashedImages: number; comparedImages: number; matchedImages: number;
  complete: boolean; hasMore: boolean; queryWidth: number; queryHeight: number;
  matches: { image: SimilarImage; pHashDistance: number; dHashDistance: number }[];
}
const maximumBytes = 16 * 1024 * 1024;
const knownErrors = new Set(["AUTH_REQUIRED", "IMAGE_TOO_LARGE", "IMAGE_PIXELS", "INVALID_IMAGE", "INVALID_UPLOAD", "SEARCH_BUSY", "SEARCH_TIMEOUT"]);

export function UploadedImageSearch() {
  const intl = useIntl(), location = useLocation();
  const text = (key: string) => intl.formatMessage({ id: `similar.upload.${key}` });
  const [selected, setSelected] = useState<{ file: File; url: string } | null>(null);
  const [distance, setDistance] = useState(4), [appliedDistance, setAppliedDistance] = useState(4);
  const [result, setResult] = useState<UploadedImageResult | null>(null);
  const [error, setError] = useState(""), [loading, setLoading] = useState(false), [dragging, setDragging] = useState(false);
  const [visible, setVisible] = useState(24);
  const request = useRef<AbortController | null>(null), generation = useRef(0);
  useEffect(() => () => { if (selected) URL.revokeObjectURL(selected.url); }, [selected]);
  useEffect(() => () => { generation.current++; request.current?.abort(); }, []);
  function invalidate() { generation.current++; request.current?.abort(); request.current = null; setLoading(false); setResult(null); setError(""); setVisible(24); }
  function choose(files: File[]) {
    if (!files.length) return;
    invalidate(); setSelected(null);
    if (files.length !== 1) { setError("ONE_IMAGE"); return; }
    const file = files[0];
    if (!file.size || file.size > maximumBytes) { setError("IMAGE_TOO_LARGE"); return; }
    if (file.type ? !["image/jpeg", "image/png", "image/webp"].includes(file.type) : !/\.(jpe?g|png|webp)$/i.test(file.name)) { setError("INVALID_IMAGE"); return; }
    setSelected({ file, url: URL.createObjectURL(file) });
  }
  async function submit(event: FormEvent) {
    event.preventDefault(); if (!selected || request.current) return;
    const controller = new AbortController(), id = ++generation.current;
    request.current = controller; setLoading(true); setResult(null); setError(""); setVisible(24); setAppliedDistance(distance);
    const body = new FormData(); body.append("file", selected.file);
    try {
      const response = await fetch(`/browse/similar-images/upload?maxDistance=${distance}`, { method: "POST", body, signal: controller.signal, credentials: "same-origin" });
      if (!response.ok) {
        const failure = await response.json().catch(() => ({})) as { code?: string };
        throw new Error(response.status === 401 ? "AUTH_REQUIRED" : failure.code || "SEARCH_FAILED");
      }
      const value = await response.json() as UploadedImageResult;
      if (!Array.isArray(value.matches)) throw new Error("SEARCH_FAILED");
      if (generation.current === id) setResult(value);
    } catch (failure) {
      if (generation.current === id && !controller.signal.aborted) { const code = failure instanceof Error ? failure.message : "SEARCH_FAILED"; setError(knownErrors.has(code) ? code : "SEARCH_FAILED"); }
    } finally {
      if (generation.current === id) { request.current = null; setLoading(false); }
    }
  }
  return <section className="similar-upload" aria-label={text("title")}>
    <p className="similar-images-help">{text("help")}</p>
    <form onSubmit={submit}>
      <div className={`similar-upload-dropzone${dragging ? " is-dragging" : ""}`} onDragOver={event => { event.preventDefault(); setDragging(true); }} onDragLeave={event => { if (!(event.relatedTarget instanceof Node) || !event.currentTarget.contains(event.relatedTarget)) setDragging(false); }} onDrop={event => { event.preventDefault(); setDragging(false); choose(Array.from(event.dataTransfer.files)); }}>
        <label className="similar-upload-file-label">{text("choose")}<input className="similar-upload-file-input" type="file" accept="image/jpeg,image/png,image/webp" onChange={event => { choose(Array.from(event.target.files || [])); event.target.value = ""; }} /></label>
        <p>{text("drop")}</p>
        {selected ? <div className="similar-upload-selection"><div className="similar-upload-preview"><img src={selected.url} alt={text("preview")} /></div><p>{selected.file.name} · {(selected.file.size / (1024 * 1024)).toFixed(2)} MiB</p><button type="button" onClick={() => { invalidate(); setSelected(null); }}>{text("remove")}</button></div> : null}
      </div>
      <div className="similar-images-form">
        <label>{intl.formatMessage({ id: "similar.distance" })}<select value={distance} onChange={event => { invalidate(); setDistance(Number(event.target.value)); }}>{[0, 2, 4, 6].map(value => <option key={value} value={value}>{value}</option>)}</select></label>
        <button type="submit" className="load-more" disabled={!selected || loading}>{text(loading ? "searching" : "search")}</button>
        {loading ? <button type="button" onClick={invalidate}>{text("cancel")}</button> : null}
      </div>
    </form>
    <details className="similar-images-rules"><summary>{intl.formatMessage({ id: "similar.rules" })}</summary><p className="similar-images-help">{text("limits")}</p><p className="similar-images-help">{text("preprocessing")}</p></details>
    {loading ? <p className="state-message" role="status">{text("searching")}</p> : null}
    {error ? <p className="state-message" role="alert">{text(`error.${error}`)}</p> : null}
    {!loading && !error && !result ? <p className="state-message">{text("idle")}</p> : null}
    {result ? <section aria-label={intl.formatMessage({ id: "similar.results" })}>
      <div className="similar-images-summary" role="status"><p>{intl.formatMessage({ id: "similar.upload.summary" }, { total: result.totalImages, hashed: result.hashedImages, compared: result.comparedImages, matches: result.matchedImages })}</p><p>{result.queryWidth} × {result.queryHeight} · pHash ≤ {appliedDistance}</p>
        {result.hashedImages < result.totalImages ? <p>{intl.formatMessage({ id: "similar.unhashed" })}</p> : null}
        {!result.complete ? <p role="alert">{intl.formatMessage({ id: "similar.incomplete" })}</p> : null}
        {result.hasMore ? <p>{text("capped")}</p> : null}
      </div>
      {!result.matches.length ? <p className="state-message">{intl.formatMessage({ id: result.complete ? "similar.empty" : "similar.partialEmpty" })}</p> : <div className="similar-upload-results">{result.matches.slice(0, visible).map(match => <article className="similar-images-pair" key={match.image.itemUUID}>
        <header><strong>pHash {match.pHashDistance} / 64 · dHash {match.dHashDistance} / 128</strong><span>{intl.formatMessage({ id: "similar.review" })}</span></header>
        <div className="similar-upload-match"><Link className="similar-images-preview" to={`/media/${encodeURIComponent(match.image.itemUUID)}`} state={{ backgroundLocation: location }} aria-label={`${intl.formatMessage({ id: "similar.open" })} · ${match.image.fileName}`}><img src={itemResourceURL(match.image.resource) ?? undefined} alt={match.image.fileName} loading="lazy" decoding="async" /></Link><p>{match.image.fileName}</p><Link to={`/gallery/${encodeURIComponent(match.image.gallerySlug)}`}>{match.image.galleryTitle}</Link></div>
      </article>)}</div>}
      {visible < result.matches.length ? <button type="button" className="load-more" onClick={() => setVisible(value => value + 24)}>{text("more")}</button> : null}
    </section> : null}
  </section>;
}
