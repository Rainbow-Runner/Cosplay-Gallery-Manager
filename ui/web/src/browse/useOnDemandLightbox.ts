import { useMutation, useQuery } from "@apollo/client/react";
import { useEffect, useState } from "react";

import { ITEM_LIGHTBOX_STATUS, REQUEST_ITEM_LIGHTBOX } from "../api/browse";
import type { GalleryMember, OnDemandResource, ResourceIdentity } from "./types";

export interface OnDemandLightboxState {
  resource?: ResourceIdentity | null;
  preparing: boolean;
  failed: boolean;
}

// Opening a static image is the explicit demand signal. The card remains
// visible immediately while this focused query follows the background job.
export function useOnDemandLightbox(item?: GalleryMember | null, enabled = true): OnDemandLightboxState {
	const eligible = enabled && item?.mediaKind === "STATIC_IMAGE";
  const existing = item?.largeResource;
  const itemUUID = item?.itemUUID ?? "";
  const [settled, setSettled] = useState(Boolean(existing));
  const [requested, setRequested] = useState<OnDemandResource | null>(null);
  const [request, requestResult] = useMutation<{ requestItemLightbox: OnDemandResource }>(REQUEST_ITEM_LIGHTBOX);
  const status = useQuery<{ itemLightboxStatus: OnDemandResource }>(ITEM_LIGHTBOX_STATUS, {
    variables: { itemUUID },
    skip: !eligible || Boolean(existing) || settled,
    fetchPolicy: "network-only",
    pollInterval: 750,
  });

  useEffect(() => {
    setSettled(Boolean(existing));
    setRequested(null);
    if (!eligible || existing || !itemUUID) return;
    let active = true;
    void request({ variables: { itemUUID } }).then(({ data }) => {
      if (!active || !data) return;
      setRequested(data.requestItemLightbox);
      if (data.requestItemLightbox.resource || data.requestItemLightbox.status === "ERROR") setSettled(true);
    }).catch(() => { if (active) setSettled(true); });
    return () => { active = false; };
  }, [eligible, existing, itemUUID, request]);

  const current = status.data?.itemLightboxStatus ?? requested;
  useEffect(() => {
    if (current?.resource || current?.status === "ERROR") setSettled(true);
  }, [current]);

  if (!eligible) return { resource: existing, preparing: false, failed: false };
  if (existing) return { resource: existing, preparing: false, failed: false };
  if (current?.resource) return { resource: current.resource, preparing: false, failed: false };
  const failed = current?.status === "ERROR" || Boolean(requestResult.error) || Boolean(status.error);
  return { resource: null, preparing: !failed, failed };
}
