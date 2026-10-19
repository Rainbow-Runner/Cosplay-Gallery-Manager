import { useEffect, type RefObject } from "react";

// Viewer and pending/error shells are mutually exclusive portals. Keeping
// their isolation lifecycle identical avoids stranded body locks on failure.
export function useViewerIsolation(focus: RefObject<HTMLElement | null>) {
  useEffect(() => {
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const overflow = document.body.style.overflow;
    const app = document.getElementById("root"), inert = app?.inert;
    if (app) app.inert = true;
    document.body.style.overflow = "hidden";
    focus.current?.focus();
    return () => { document.body.style.overflow = overflow; if (app) app.inert = inert ?? false; if (previous?.isConnected) previous.focus(); };
  }, [focus]);
}
