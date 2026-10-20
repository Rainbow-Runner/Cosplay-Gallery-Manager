import { useEffect, useState } from "react";

// Only the focused static image is decoded. Keep the preview until that exact
// identity/source is ready; stale responses may never paint over a new item.
export function useDecodedImage(source: string | null, preview: string | null, identity: string, enabled: boolean) {
  const [attempt, setAttempt] = useState(0);
  const [settled, setSettled] = useState<{identity: string; source: string; attempt: number; failed: boolean} | null>(null);
  const needsDecode = enabled && !!source && source !== preview;
  const current = settled?.identity === identity && settled.source === source && settled.attempt === attempt ? settled : null;
  useEffect(() => {
    if (!needsDecode || !source) return;
    let active = true;
    const image = new Image();
    image.decoding = "async";
    const loaded = typeof image.decode === "function" ? null : new Promise<void>((resolve,reject) => { image.onload = () => resolve(); image.onerror = () => reject(new Error("Image load failed")); });
    image.src = source;
    void (loaded ?? image.decode()).then(() => {
      if (active) setSettled({identity,source,attempt,failed:false});
    }).catch(() => { if (active) setSettled({identity,source,attempt,failed:true}); });
    return () => { active = false; image.onload = null; image.onerror = null; image.src = ""; };
  }, [needsDecode,source,identity,attempt]);
  return {
    url: !enabled ? source ?? preview : !needsDecode || current && !current.failed ? source ?? preview : preview,
    preparing: needsDecode && !current,
    failed: needsDecode && !!current?.failed,
    retry: () => setAttempt(value=>value+1),
  };
}
