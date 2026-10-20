import { useEffect, useMemo, useState } from "react";

import type { GalleryMember } from "./types";
import { itemResourceURL } from "./resourceUrl";
import { useOnDemandLightbox } from "./useOnDemandLightbox";

export function originalImageURL(item: GalleryMember) {
  const revision = item.largeResource?.contentRevision ?? item.cardResource?.contentRevision;
  if (!revision) return null;
  return `/resource/image/${encodeURIComponent(item.itemUUID)}/${revision}/original`;
}

// The authenticated HEAD request lets the server make the MIME/dimension/size
// decision. Static images that are not eligible transparently fall back to the
// existing on-demand 4096 proxy; animated images retain their exact source.
export function useImageDisplay(item?: GalleryMember | null) {
  const eligible = item?.mediaKind === "STATIC_IMAGE" || item?.mediaKind === "ANIMATED_IMAGE";
  const candidate = useMemo(() => item && eligible ? originalImageURL(item) : null, [eligible, item]);
  const [probe, setProbe] = useState<{candidate: string | null; accepted: boolean} | null>(null);
  const probeComplete = !candidate || probe?.candidate === candidate;
  const directURL = candidate && probe?.candidate === candidate && probe.accepted ? candidate : null;
  const probeFailed = probeComplete && !!candidate && !directURL;
  const useProxy = Boolean(item?.mediaKind === "STATIC_IMAGE" && probeComplete && !directURL);
  const proxy = useOnDemandLightbox(item, useProxy);

  useEffect(() => {
    if (!candidate) return;
    const controller = new AbortController();
    void fetch(candidate, { method: "HEAD", credentials: "same-origin", signal: controller.signal }).then((response) => {
      if (!controller.signal.aborted) setProbe({candidate,accepted:response.ok});
    }).catch((error: unknown) => {
      if (error instanceof DOMException && error.name === "AbortError") return;
      if (!controller.signal.aborted) setProbe({candidate,accepted:false});
    });
    return () => controller.abort();
  }, [candidate]);

  const proxyURL = itemResourceURL(proxy.resource ?? item?.largeResource ?? item?.cardResource);
  const animatedFailure = item?.mediaKind === "ANIMATED_IMAGE" && probeComplete && probeFailed;
  return {
    url: directURL ?? (item?.mediaKind === "STATIC_IMAGE" && probeComplete ? proxyURL : null),
    preparing: Boolean(eligible && !probeComplete) || (useProxy && proxy.preparing),
    failed: Boolean(animatedFailure || (useProxy && proxy.failed)),
    mode: directURL ? "DIRECT" as const : "PROXY" as const,
  };
}
