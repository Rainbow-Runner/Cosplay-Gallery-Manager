import type { Location } from "react-router-dom";

export function viewerBackgroundLocation(location: Location): Location | undefined {
  if (!location.pathname.startsWith("/media/")) return;
  const candidate = (location.state as { backgroundLocation?: Location } | null)?.backgroundLocation;
  if (!candidate || typeof candidate.pathname !== "string" || !/^\/(?:random|favorites|history|similar-images|gallery)(?:\/|$)/.test(candidate.pathname)) return;
  return candidate;
}
