import { useMutation, useQuery } from "@apollo/client/react";
import { useCallback, useEffect, useState } from "react";

import { ITEM_VIDEO_PLAYBACK_STATUS, REQUEST_ITEM_VIDEO_PLAYBACK } from "../api/browse";
import type { GalleryMember, VideoPlaybackStatus } from "./types";
import { itemResourceURL } from "./resourceUrl";

export interface OnDemandVideoPlaybackState {
  url: string | null;
  mode: VideoPlaybackStatus["mode"] | "HLS_SESSION" | "MP4_PROXY";
  preparing: boolean;
  failed: boolean;
  errorCode: string;
  progress?: { percent: number; speed: number; etaSeconds: number };
  hls?: boolean;
  retry: () => void;
  onPlaybackError: () => void;
}

function directVideoURL(itemUUID: string, revision: number): string {
  return `/resource/video/${encodeURIComponent(itemUUID)}/${revision}/direct`;
}

// The mounted focused viewer is the demand signal. Gallery cards, member
// grids, scrubbers, and adjacent Lightbox items never mount this hook.
export function useLegacyVideoPlayback(item?: GalleryMember | null): OnDemandVideoPlaybackState {
  const eligible = item?.mediaKind === "VIDEO";
  const itemUUID = item?.itemUUID ?? "";
  const [settled, setSettled] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const [requested, setRequested] = useState<VideoPlaybackStatus | null>(null);
  const [browserErrorFor, setBrowserErrorFor] = useState("");
  const onPlaybackError = useCallback(() => setBrowserErrorFor(itemUUID), [itemUUID]);
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
    setBrowserErrorFor("");
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

  if (!eligible) return { url: null, mode: "", preparing: false, failed: false, errorCode: "", retry, onPlaybackError };
  if (browserErrorFor === itemUUID && itemUUID) return { url: null, mode: current?.mode ?? "", preparing: false, failed: true, errorCode: "VIDEO_BROWSER_PLAYBACK_FAILED", retry, onPlaybackError };
  if (current?.status === "READY" && current.mode === "DIRECT") {
    return { url: directVideoURL(itemUUID, current.contentRevision), mode: current.mode, preparing: false, failed: false, errorCode: "", retry, onPlaybackError };
  }
  if (current?.status === "READY" && current.resource) {
    return { url: itemResourceURL(current.resource), mode: current.mode, preparing: false, failed: false, errorCode: "", retry, onPlaybackError };
  }
  const failed = current?.status === "ERROR" || Boolean(requestResult.error) || Boolean(status.error);
  return { url: null, mode: current?.mode ?? "", preparing: !failed, failed, errorCode: current?.errorCode ?? "", retry, onPlaybackError };
}

interface ProgressiveResponse {
  mode: OnDemandVideoPlaybackState["mode"];
  status: "PENDING" | "PROCESSING" | "STREAMING" | "READY" | "ERROR";
  url?: string;
  lease?: string;
  errorCode?: string;
  progress?: OnDemandVideoPlaybackState["progress"];
}

// Only a mounted viewer requests a progressive session. A lease heartbeat
// keeps the active stream alive; unmount and playback errors release it.
export function useOnDemandVideoPlayback(item?: GalleryMember | null): OnDemandVideoPlaybackState {
  const eligible = item?.mediaKind === "VIDEO";
  const itemUUID = item?.itemUUID ?? "";
  const [attempt, setAttempt] = useState(0);
  const [fallback, setFallback] = useState(false);
  const [progressive, setProgressive] = useState<ProgressiveResponse | null>(null);
  const [fatal, setFatal] = useState("");
  const legacy = useLegacyVideoPlayback(fallback ? item : null);

  useEffect(() => {
    if (!eligible || !itemUUID || fallback) return;
    let active = true;
    let lease = "";
    let timer: ReturnType<typeof setTimeout> | undefined;
    const release = () => { if (lease) void fetch(`/playback/session/${lease}/release`, { method: "POST", credentials: "same-origin" }).catch(() => {}); };
    const poll = async () => {
      if (!active || !lease) return;
      try {
        const response = await fetch(`/playback/session/${lease}/status`, { credentials: "same-origin" });
        if (response.status === 401 || response.status === 403) { setFatal("VIDEO_PLAYBACK_UNAUTHORIZED"); return; }
        if (!response.ok) throw new Error("session lost");
        const next = await response.json() as ProgressiveResponse;
        if (!active) return;
        setProgressive((previous) => ({ ...next, url: previous?.url, lease }));
        if (next.status === "ERROR") { release(); setFallback(true); return; }
        timer = setTimeout(() => { void poll(); }, next.status === "READY" ? 10_000 : 1_000);
      } catch { if (active) { release(); setFallback(true); } }
    };
    void fetch(`/playback/video/${encodeURIComponent(itemUUID)}`, { method: "POST", credentials: "same-origin" }).then(async (response) => {
      if (response.status === 401 || response.status === 403) { if (active) setFatal("VIDEO_PLAYBACK_UNAUTHORIZED"); return; }
      if (!response.ok) throw new Error("progressive unavailable");
      const next = await response.json() as ProgressiveResponse;
      if (!active) { if (next.lease) void fetch(`/playback/session/${next.lease}/release`, { method: "POST", credentials: "same-origin" }); return; }
      if (next.status === "ERROR") { setFatal(next.errorCode ?? "VIDEO_PLAYBACK_UNAVAILABLE"); return; }
      lease = next.lease ?? "";
      setProgressive(next);
      if (lease) timer = setTimeout(() => { void poll(); }, 1_000);
    }).catch(() => { if (active) setFallback(true); });
    return () => { active = false; if (timer) clearTimeout(timer); release(); };
  }, [attempt, eligible, fallback, itemUUID]);

  const retry = useCallback(() => { setFallback(false); setFatal(""); setProgressive(null); setAttempt((value) => value + 1); }, []);
  const onPlaybackError = useCallback(() => {
    if (progressive?.mode === "HLS_SESSION") setFallback(true);
    else setFatal("VIDEO_BROWSER_PLAYBACK_FAILED");
  }, [progressive?.mode]);
  if (!eligible) return { url: null, mode: "", preparing: false, failed: false, errorCode: "", retry, onPlaybackError };
  if (fallback) return { ...legacy, retry };
  if (fatal) return { url: null, mode: progressive?.mode ?? "", preparing: false, failed: true, errorCode: fatal, retry, onPlaybackError };
  return { url: progressive?.url ?? null, mode: progressive?.mode ?? "", hls: progressive?.mode === "HLS_SESSION",
    preparing: !progressive || progressive.status === "PENDING" || progressive.status === "PROCESSING",
    failed: false, errorCode: "", progress: progressive?.progress, retry, onPlaybackError };
}
