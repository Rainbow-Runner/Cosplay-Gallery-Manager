import { useMutation, useQuery } from "@apollo/client/react";
import { useEffect, useState } from "react";

import { ITEM_ANIMATED_PREVIEW_STATUS, REQUEST_ITEM_ANIMATED_PREVIEW } from "../api/browse";
import type { GalleryMember, OnDemandResource, ResourceIdentity } from "./types";

export interface OnDemandAnimatedPreviewState {
  resource?: ResourceIdentity | null;
  preparing: boolean;
  failed: boolean;
}

export function useOnDemandAnimatedPreview(item: GalleryMember, enabled: boolean): OnDemandAnimatedPreviewState {
  const eligible = enabled && item.mediaKind === "ANIMATED_IMAGE";
  const [settled, setSettled] = useState(false);
  const [requested, setRequested] = useState<OnDemandResource | null>(null);
  const [request, requestResult] = useMutation<{ requestItemAnimatedPreview: OnDemandResource }>(REQUEST_ITEM_ANIMATED_PREVIEW);
  const status = useQuery<{ itemAnimatedPreviewStatus: OnDemandResource }>(ITEM_ANIMATED_PREVIEW_STATUS, {
    variables: { itemUUID: item.itemUUID },
    skip: !eligible || settled,
    fetchPolicy: "network-only",
    pollInterval: 750,
  });

  useEffect(() => {
    setSettled(false);
    setRequested(null);
    if (!eligible) return;
    let active = true;
    void request({ variables: { itemUUID: item.itemUUID } }).then(({ data }) => {
      if (!active || !data) return;
      setRequested(data.requestItemAnimatedPreview);
      if (data.requestItemAnimatedPreview.resource || data.requestItemAnimatedPreview.status === "ERROR") setSettled(true);
    }).catch(() => { if (active) setSettled(true); });
    return () => { active = false; };
  }, [eligible, item.itemUUID, request]);

  const current = status.data?.itemAnimatedPreviewStatus ?? requested;
  useEffect(() => {
    if (current?.resource || current?.status === "ERROR") setSettled(true);
  }, [current]);
  if (!eligible) return { resource: null, preparing: false, failed: false };
  if (current?.resource) return { resource: current.resource, preparing: false, failed: false };
  const failed = current?.status === "ERROR" || Boolean(requestResult.error) || Boolean(status.error);
  return { resource: null, preparing: !failed, failed };
}
