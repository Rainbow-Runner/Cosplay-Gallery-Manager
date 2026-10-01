import { useEffect, useRef } from "react";

export function PlaybackVideo({ url, hls, autoPlay = false, onError }: {
  url: string; hls?: boolean; autoPlay?: boolean; onError: () => void;
}) {
  const ref = useRef<HTMLVideoElement>(null);
  const errorRef = useRef(onError);
  errorRef.current = onError;
  useEffect(() => {
    const video = ref.current;
    if (!video) return;
    let cancelled = false;
    let destroy: (() => void) | undefined;
    if (!hls || video.canPlayType("application/vnd.apple.mpegurl")) {
      video.src = url;
    } else {
      void import("hls.js").then(({ default: Hls }) => {
        if (cancelled) return;
        if (!Hls.isSupported()) { errorRef.current(); return; }
        const player = new Hls({ enableWorker: false, maxBufferLength: 24, backBufferLength: 12,
          maxBufferSize: 64 * 1024 * 1024 });
        player.on(Hls.Events.ERROR, (_, data) => { if (data.fatal) errorRef.current(); });
        player.loadSource(url);
        player.attachMedia(video);
        destroy = () => player.destroy();
      }).catch(() => { if (!cancelled) errorRef.current(); });
    }
    return () => {
      cancelled = true;
      destroy?.();
      video.pause();
      video.removeAttribute("src");
      video.load();
    };
  }, [hls, url]);
  return <video ref={ref} controls playsInline autoPlay={autoPlay} onError={() => errorRef.current()} />;
}
