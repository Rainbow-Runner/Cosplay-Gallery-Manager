import { useApolloClient, useQuery } from "@apollo/client/react";
import { useCallback, useEffect, useRef, useState } from "react";

import { BROWSE_UI_SETTINGS, GALLERY_COVER_STATUS, REQUEST_GALLERY_COVER } from "../api/browse";
import { itemResourceURL } from "./resourceUrl";
import type { BrowseGalleryCard, BrowseUISettings, OnDemandResource, ResourceIdentity } from "./types";

export type CoverVariant = "CARD_480" | "CARD_960" | "CARD_1600";

// object-fit:cover may upscale landscape images much more than portrait ones.
export function galleryCoverVariant(width: number, height: number, imageWidth: number, imageHeight: number, dpr: number): CoverVariant {
  if (![width, height, imageWidth, imageHeight, dpr].every(n => Number.isFinite(n) && n > 0)) return "CARD_480";
  const required = Math.max(imageWidth, imageHeight) * Math.max(width / imageWidth, height / imageHeight) * dpr;
  return required <= 480 ? "CARD_480" : required <= 960 ? "CARD_960" : "CARD_1600";
}

export function useGalleryCover(card: BrowseGalleryCard) {
  const client = useApolloClient();
  const settings = useQuery<{ browseUISettings: BrowseUISettings }>(BROWSE_UI_SETTINGS, { fetchPolicy: "cache-and-network" });
  const allow1600 = settings.data?.browseUISettings.galleryCover1600Enabled === true;
  const frame = useRef<HTMLElement>(null);
  const base = card.cover.resource;
  const baseURL = itemResourceURL(base);
  const identity = `${card.setID}:${card.cover.revision}:${baseURL}`;
  const [visible, setVisible] = useState(false);
  const [geometry, setGeometry] = useState({ width: 0, height: 0, dpr: 1 });
  const [natural, setNatural] = useState({ identity: "", width: 0, height: 0 });
  const [enhanced, setEnhanced] = useState<{ key: string; resource: ResourceIdentity } | null>(null);
  const [broken, setBroken] = useState("");
  const [brokenBase, setBrokenBase] = useState("");
  const displayBase = base?.variant === "CARD_DISPLAY_960";
  const eligible = (base?.variant === "CARD_480" || displayBase) && card.cover.kind === "ITEM";
  const requiredVariant = natural.identity === identity ? galleryCoverVariant(geometry.width, geometry.height, natural.width, natural.height, geometry.dpr) : "CARD_480";
  const cappedVariant = !allow1600 && requiredVariant === "CARD_1600" ? "CARD_960" : requiredVariant;
  const variant = displayBase && cappedVariant === "CARD_1600" ? "CARD_DISPLAY_1600" : cappedVariant;
  const requestKey = `${identity}:${variant}`;

  useEffect(() => {
    const element = frame.current;
    if (!element) return;
    const measure = () => {
      const bounds = element.querySelector(".gallery-card__poster")?.getBoundingClientRect();
      if (bounds) setGeometry({ width: bounds.width, height: bounds.height, dpr: window.devicePixelRatio || 1 });
    };
    measure();
    const resize = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(measure);
    resize?.observe(element);
    const intersection = typeof IntersectionObserver === "undefined" ? null : new IntersectionObserver(entries => setVisible(entries.some(e => e.isIntersecting)), { rootMargin: "200px" });
    if (intersection) intersection.observe(element); else setVisible(true);
    window.addEventListener("resize", measure);
    // DPR can change on zoom or when moving the window between monitors.
    let media: MediaQueryList | null = null;
    const dprChanged = () => { measure(); watchDPR(); };
    const watchDPR = () => {
      media?.removeEventListener?.("change", dprChanged);
      media = typeof window.matchMedia === "function" ? window.matchMedia(`(resolution: ${window.devicePixelRatio || 1}dppx)`) : null;
      media?.addEventListener?.("change", dprChanged);
    };
    watchDPR();
    return () => { resize?.disconnect(); intersection?.disconnect(); window.removeEventListener("resize", measure); media?.removeEventListener?.("change", dprChanged); };
  }, []);

  useEffect(() => {
    if (!eligible || !visible || variant === "CARD_480" || (displayBase && variant !== "CARD_DISPLAY_1600") || broken === requestKey || !base) return;
    let active = true;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let polls = 0;
    const variables = { setID: card.setID, itemUUID: base.itemUUID, variant };
    const accept = (value?: OnDemandResource | null) => {
      if (!active) return;
      if (value?.resource && value.resource.itemUUID === base.itemUUID && value.resource.contentRevision === base.contentRevision && value.resource.variant === variant) {
        setEnhanced({ key: requestKey, resource: value.resource });
      } else if (value?.status !== "ERROR" && ++polls <= 40) {
        timer = setTimeout(() => {
          if (!active) return;
          void client.query<{ galleryCoverStatus: OnDemandResource }>({ query: GALLERY_COVER_STATUS, variables, fetchPolicy: "network-only" })
            .then(({ data }) => accept(data?.galleryCoverStatus)).catch(() => {});
        }, 2000);
      }
    };
    void client.mutate<{ requestGalleryCover: OnDemandResource }>({ mutation: REQUEST_GALLERY_COVER, variables })
      .then(({ data }) => accept(data?.requestGalleryCover)).catch(() => {});
    return () => { active = false; if (timer) clearTimeout(timer); };
  }, [client, eligible, visible, requestKey, variant, broken, card.setID, base?.itemUUID, base?.contentRevision]);

  const onLoad = useCallback((image: HTMLImageElement) => {
    if (image.naturalWidth > 0 && image.naturalHeight > 0) setNatural({ identity, width: image.naturalWidth, height: image.naturalHeight });
  }, [identity]);
  const usingEnhanced = eligible && enhanced?.key === requestKey && broken !== requestKey;
  const onError = useCallback(() => {
    if (usingEnhanced) setBroken(requestKey); else if (displayBase) setBrokenBase(identity);
  }, [usingEnhanced, requestKey, displayBase, identity]);
  const resource = usingEnhanced ? enhanced!.resource : brokenBase === identity && card.cover.fallbackResource ? card.cover.fallbackResource : base;
  return { frame, coverURL: itemResourceURL(resource), onLoad, onError };
}
