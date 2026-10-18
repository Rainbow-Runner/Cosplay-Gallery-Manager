import { useApolloClient } from "@apollo/client/react";
import { useCallback, useEffect, useRef, useState } from "react";
import { useIntl } from "react-intl";
import { COSER_TIMELINE } from "../api/browse";
import { GalleryCard } from "./GalleryCard";
import type { BrowseGalleryCard, Scope } from "./types";

export interface TimelineEntry { month: string; card: BrowseGalleryCard }
interface TimelineBatch { items: TimelineEntry[]; endCursor: string; hasNextPage: boolean }

export function groupTimelineMonths(entries: TimelineEntry[]) {
  const groups: { month: string; cards: BrowseGalleryCard[] }[] = [];
  const seen = new Set<string>();
  for (const { month, card } of entries) {
    if (seen.has(card.setID)) continue;
    seen.add(card.setID);
    let group = groups[groups.length - 1];
    if (!group || group.month !== month) { group = { month, cards: [] }; groups.push(group); }
    group.cards.push(card);
  }
  return groups;
}

export function estimateTimelineBatch(width: number, height: number, viewportWidth: number) {
  const columns = viewportWidth >= 1536 ? 5 : viewportWidth >= 1024 ? 4 : viewportWidth >= 768 ? 3 : 2;
  const cardWidth = Math.max(1, (width - (columns - 1) * 16) / columns);
  const rows = Math.max(1, Math.ceil(Math.max(0, height) / (cardWidth * 4 / 3 + 90)));
  return Math.min(24, Math.max(4, columns * (rows + 1)));
}

export function CoserMonthTimeline({ coserUUID, scope, date }: { coserUUID: string; scope: Scope; date: string }) {
  const client = useApolloClient();
  const intl = useIntl();
  const [entries, setEntries] = useState<TimelineEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [failed, setFailed] = useState(false);
  const [hasNext, setHasNext] = useState(true);
  const [started, setStarted] = useState(false);
  const root = useRef<HTMLDivElement>(null);
  const sentinel = useRef<HTMLDivElement>(null);
  const request = useRef<AbortController | null>(null);
  const cursor = useRef<string | null>(null);
  const next = useRef(true);
  const disposed = useRef(false);
  const failure = useRef(false);

  const load = useCallback(async () => {
    if (request.current || !next.current || disposed.current) return;
    const controller = new AbortController();
    request.current = controller;
    setLoading(true); setFailed(false); failure.current = false;
    const box = root.current?.getBoundingClientRect();
    const first = cursor.current ? 24 : estimateTimelineBatch(Math.max(1, (box?.width ?? window.innerWidth) - 100), window.innerHeight - (box?.top ?? 0), window.innerWidth);
    try {
      const result = await client.query<{ coserTimeline: TimelineBatch }>({
        query: COSER_TIMELINE, variables: { coserUUID, scope, date, first, after: cursor.current },
        fetchPolicy: "no-cache", context: { queryDeduplication: false, fetchOptions: { signal: controller.signal } },
      });
      if (disposed.current || controller.signal.aborted) return;
      const batch = result.data?.coserTimeline;
      if (!batch || (batch.hasNextPage && (!batch.endCursor || batch.endCursor === cursor.current))) throw new Error("Invalid timeline continuation");
      cursor.current = batch.endCursor;
      next.current = batch.hasNextPage;
      setEntries((previous) => [...previous, ...batch.items]);
      setHasNext(batch.hasNextPage); setStarted(true);
    } catch {
      if (!disposed.current && !controller.signal.aborted) { setFailed(true); failure.current = true; }
    } finally {
      if (request.current === controller) {
        request.current = null;
        if (!disposed.current) setLoading(false);
      }
    }
  }, [client, coserUUID, scope, date]);

  useEffect(() => {
    disposed.current = false;
    void load();
    return () => { disposed.current = true; request.current?.abort(); request.current = null; };
  }, [load]);

  useEffect(() => {
    if (loading || failed || !hasNext || !started || !sentinel.current) return;
    // Short first batches and viewport resizes must not leave an empty screen.
    const fill = () => { if (!failure.current && sentinel.current && sentinel.current.getBoundingClientRect().top < window.innerHeight + 250) void load(); };
    fill();
    window.addEventListener("resize", fill);
    const observer = typeof IntersectionObserver === "undefined" ? null : new IntersectionObserver((changes) => {
      if (changes.some((entry) => entry.isIntersecting) && !failure.current) void load();
    }, { rootMargin: "250px 0px" });
    observer?.observe(sentinel.current);
    return () => { observer?.disconnect(); window.removeEventListener("resize", fill); };
  }, [loading, failed, hasNext, started, entries, load]);

  return <div className="coser-month-timeline" ref={root}>
    <ol className="coser-month-timeline__months">{groupTimelineMonths(entries).map(({ month, cards }) => {
      const label = intl.formatDate(new Date(`${month}-01T00:00:00Z`), { year: "numeric", month: "short", timeZone: "UTC" });
      return <li className="coser-month-timeline__month" key={month}>
        <h2 className="coser-month-timeline__date"><time dateTime={month}>{label}</time></h2>
        <section className="gallery-grid" aria-label={label}>{cards.map((card) => <GalleryCard key={card.setID} card={card} scrubberEnabled={false} />)}</section>
      </li>;
    })}</ol>
    {!entries.length && started && !hasNext ? <p className="state-message">{intl.formatMessage({ id: "state.empty" })}</p> : null}
    <div className="coser-month-timeline__sentinel" ref={sentinel}>
      {loading ? <p role="status">{intl.formatMessage({ id: "state.loading" })}</p> : null}
      {failed ? <p role="alert">{intl.formatMessage({ id: "state.error" })}</p> : null}
      {hasNext && !loading ? <button type="button" onClick={() => void load()}>{intl.formatMessage({ id: failed ? "timeline.retry" : "timeline.loadMore" })}</button> : null}
      {entries.length > 0 && !hasNext ? <p role="status">{intl.formatMessage({ id: "timeline.end" })}</p> : null}
    </div>
  </div>;
}
