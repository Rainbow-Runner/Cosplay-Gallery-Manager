import { type PointerEvent, useEffect, useRef, useState } from "react";

interface Props {
  coverURL: string | null;
  previewCount: number;
  previewURL: (ordinal: number) => string;
  enabled: boolean;
  alt: string;
}

function preciseHoverAvailable() {
  return typeof window.matchMedia === "function" &&
    window.matchMedia("(hover: hover) and (pointer: fine)").matches;
}

export function GalleryCardPosterScrubber({ coverURL, previewCount, previewURL, enabled, alt }: Props) {
  const [ordinal, setOrdinal] = useState<number | null>(null);
  const [previewSource, setPreviewSource] = useState<string | null>(null);
  const frame = useRef<number | null>(null);
  const activeRequest = useRef<AbortController | null>(null);
  const visited = useRef(new Map<number, string>());

  useEffect(() => {
    if (ordinal === null) {
      activeRequest.current?.abort();
      setPreviewSource(null);
      return;
    }
    const cached = visited.current.get(ordinal);
    if (cached) {
      setPreviewSource(cached);
      return;
    }
    activeRequest.current?.abort();
    const controller = new AbortController();
    activeRequest.current = controller;
    void fetch(previewURL(ordinal), { credentials: "same-origin", signal: controller.signal })
      .then((response) => {
        if (!response.ok) throw new Error(`preview ${response.status}`);
        return response.blob();
      })
      .then((blob) => {
        if (controller.signal.aborted) return;
        const objectURL = URL.createObjectURL(blob);
        visited.current.set(ordinal, objectURL);
        setPreviewSource(objectURL);
      })
      .catch((error: unknown) => {
        if (!(error instanceof DOMException && error.name === "AbortError")) setPreviewSource(null);
      });
    return () => controller.abort();
  }, [ordinal, previewURL]);

  useEffect(() => () => {
    activeRequest.current?.abort();
    for (const value of visited.current.values()) URL.revokeObjectURL(value);
  }, []);

  function onPointerMove(event: PointerEvent<HTMLDivElement>) {
    if (!enabled || previewCount < 1 || event.pointerType !== "mouse" || !preciseHoverAvailable()) return;
    const bounds = event.currentTarget.getBoundingClientRect();
    if (bounds.width <= 0) return;
    const ratio = Math.max(0, Math.min(0.999999, (event.clientX - bounds.left) / bounds.width));
    const next = Math.floor(ratio * previewCount);
    if (frame.current !== null) cancelAnimationFrame(frame.current);
    frame.current = requestAnimationFrame(() => {
      setOrdinal(next);
      frame.current = null;
    });
  }

  function restoreCover() {
    if (frame.current !== null) cancelAnimationFrame(frame.current);
    frame.current = null;
    setOrdinal(null);
  }

  const source = ordinal === null ? coverURL : previewSource ?? coverURL;
  return (
    <div className="poster-scrubber" onPointerMove={onPointerMove} onPointerLeave={restoreCover}>
      {source ? <img src={source} alt={alt} draggable={false} /> : <div className="poster-scrubber__placeholder" role="img" aria-label={alt} />}
      {enabled && previewCount > 1 && ordinal !== null ? (
        <div className="poster-scrubber__axis" aria-hidden="true">
          <span style={{ width: `${((ordinal + 1) / previewCount) * 100}%` }} />
        </div>
      ) : null}
    </div>
  );
}
