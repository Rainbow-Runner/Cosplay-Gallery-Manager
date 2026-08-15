import { useMutation, useQuery } from "@apollo/client/react";
import { useCallback, useEffect, useState } from "react";

import { ITEM_VIDEO_PLAYBACK_STATUS, REQUEST_ITEM_VIDEO_PLAYBACK } from "../api/browse";
import type { GalleryMember, VideoPlaybackStatus } from "./types";
import { itemResourceURL } from "./resourceUrl";

export interface OnDemandVideoPlaybackState {
  url: string | null;
  mode: VideoPlaybackStatus["mode"];
  preparing: boolean;
  failed: boolean;
  errorCode: string;
  retry: () => void;
}

function directVideoURL(itemUUID: string, revision: number): string {
  return `/resource/video/${encodeURIComponent(itemUUID)}/${revision}/direct`;
}

// The mounted focused viewer is the demand signal. Gallery cards, member
// grids, scrubbers, and adjacent Lightbox items never mount this hook.
export function useOnDemandVideoPlayback(item?: GalleryMember | null): OnDemandVideoPlaybackState {
  const eligible = item?.mediaKind === "VIDEO";
  const itemUUID = item?.itemUUID ?? "";
  const [settled, setSettled] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const [requested, setRequested] = useState<VideoPlaybackStatus | null>(null);
  const [request, requestResult] = useMutation<{ requestItemVideoPlayback: VideoPlaybackStatus }>(REQUEST_ITEM_VIDEO_PLAYBACK);
  const status = useQuery<{ itemVideoPlaybackStatus: VideoPlaybackStatus }>(ITEM_VIDEO_PLAYBACK_STATUS, {
    variables: { itemUUID },
    skip: !eligible || !itemUUID || settled,
    fetchPolicy: "network-only",
    pollInterval: 750,
  });

  useEffect(() => {
    setSettled(false);
    setRequested(null);
    if (!eligible || !itemUUID) return;
    let active = true;
    void request({ variables: { itemUUID } }).then(({ data }) => {
      if (!active || !data) return;
      const result = data.requestItemVideoPlayback;
      setRequested(result);
      if (result.status === "READY" || result.status === "ERROR") setSettled(true);
    }).catch(() => { if (active) setSettled(true); });
    return () => { active = false; };
  }, [attempt, eligible, itemUUID, request]);

  const queried = status.data?.itemVideoPlaybackStatus;
  const current = queried?.itemUUID === itemUUID ? queried : requested?.itemUUID === itemUUID ? requested : null;
  useEffect(() => {
    if (current?.status === "READY" || current?.status === "ERROR") setSettled(true);
  }, [current]);

  const retry = useCallback(() => {
    setSettled(false);
    setRequested(null);
    setAttempt((value) => value + 1);
  }, []);

  if (!eligible) return { url: null, mode: "", preparing: false, failed: false, errorCode: "", retry };
  if (current?.status === "READY" && current.mode === "DIRECT") {
    return { url: directVideoURL(itemUUID, current.contentRevision), mode: current.mode, preparing: false, failed: false, errorCode: "", retry };
  }
  if (current?.status === "READY" && current.resource) {
    return { url: itemResourceURL(current.resource), mode: current.mode, preparing: false, failed: false, errorCode: "", retry };
  }
  const failed = current?.status === "ERROR" || Boolean(requestResult.error) || Boolean(status.error);
  return { url: null, mode: current?.mode ?? "", preparing: !failed, failed, errorCode: current?.errorCode ?? "", retry };
}
