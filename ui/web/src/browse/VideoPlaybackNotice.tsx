import { useIntl } from "react-intl";
import type { OnDemandVideoPlaybackState } from "./useOnDemandVideoPlayback";

const reasons: Record<string, string> = {
  ARCHIVE_VIDEO_COMPRESSED: "video.archiveCompressed",
  ARCHIVE_VIDEO_DIRECT_UNAVAILABLE: "video.archiveDirectUnavailable",
  ARCHIVE_VIDEO_UNSAFE: "video.archiveUnsafe",
  ARCHIVE_VIDEO_LIMIT: "video.archiveLimit",
  ARCHIVE_VIDEO_SOURCE_CHANGED: "video.archiveSourceChanged",
  ARCHIVE_VIDEO_CODEC_UNSUPPORTED: "video.archiveCodec",
};

export function VideoPlaybackNotice({ state, className }: { state: OnDemandVideoPlaybackState; className: string }) {
  const intl = useIntl();
  if (state.preparing) return <span className={className}>{intl.formatMessage({ id: "video.preparing" })}</span>;
  if (!state.failed) return null;
  const archiveFailure = state.errorCode.startsWith("ARCHIVE_VIDEO_");
  return <span className={className} role="alert">
    {intl.formatMessage({ id: reasons[state.errorCode] ?? "video.unavailable" })}
    {state.errorCode ? ` (${state.errorCode})` : ""}
    {archiveFailure ? <> {intl.formatMessage({ id: "video.archiveManualExtract" })}</> : <> <button type="button" onClick={state.retry}>{intl.formatMessage({ id: "video.retry" })}</button></>}
  </span>;
}
