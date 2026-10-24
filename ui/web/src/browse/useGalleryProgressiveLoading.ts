import { useEffect, useLayoutEffect, useRef, useState, type RefObject } from "react";
import { justifiedTargetHeight, type GalleryMediaLayout } from "./galleryMediaLayout";

export function estimateMediaBatch(width: number, height: number, layout: GalleryMediaLayout) {
  const columns = Math.max(1, Math.floor((Math.max(1, width) + 12) / 220));
  const rowHeight = layout === "JUSTIFIED" ? justifiedTargetHeight(width) : Math.max(160, width / columns * 4 / 3);
  return Math.min(48, Math.max(4, columns * (Math.max(1, Math.ceil(Math.max(0, height) / rowHeight)) + 1)));
}

export function useGalleryProgressiveLoading({ enabled, identity, total, root, layout, paused }: {
  enabled: boolean; identity: string; total: number; root: RefObject<HTMLDivElement | null>; layout: GalleryMediaLayout; paused: boolean;
}) {
  const [budget, setBudget] = useState(24);
  const sentinel = useRef<HTMLDivElement>(null);
  const pending = useRef(false);
  const seeded = useRef("");
  useLayoutEffect(() => {
    if (!enabled) { seeded.current="";return; }
    if(seeded.current===identity)return;
    seeded.current=identity;
    const box = root.current?.getBoundingClientRect();
    setBudget(estimateMediaBatch(box?.width || window.innerWidth, window.innerHeight - (box?.top || 0), layout));
  }, [enabled, identity, root, layout]);
  useEffect(() => {
    if (!enabled || paused || budget >= total || !sentinel.current) return;
    let frame = 0;
    const fill = () => {
      if (document.hidden || pending.current || !sentinel.current || sentinel.current.getBoundingClientRect().top > window.innerHeight + 250) return;
      pending.current = true;
      frame = requestAnimationFrame(() => { pending.current = false; setBudget(n => Math.min(total, n + 24)); });
    };
    fill();
    const observer = typeof IntersectionObserver === "undefined" ? null : new IntersectionObserver(entries => {
      if (entries.some(e => e.isIntersecting)) fill();
    }, { rootMargin: "250px 0px" });
    observer?.observe(sentinel.current);
    window.addEventListener("resize", fill);
    document.addEventListener("visibilitychange", fill);
    return () => { observer?.disconnect(); cancelAnimationFrame(frame); pending.current = false; window.removeEventListener("resize", fill); document.removeEventListener("visibilitychange", fill); };
  }, [enabled, paused, total, budget, identity]);
  return { budget, sentinel, loadMore: () => setBudget(n => Math.min(total, n + 24)), reveal: (index: number) => setBudget(n => Math.max(n, index + 1)) };
}
