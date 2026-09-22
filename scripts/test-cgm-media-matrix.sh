#!/usr/bin/env bash
set -euo pipefail

cgm_go="${CGM_GO:-go}"
cgm_ffmpeg="${CGM_FFMPEG_BIN:-ffmpeg}"
cgm_libraw="${CGM_LIBRAW_BIN:-dcraw}"

for executable in "${cgm_go}" "${cgm_ffmpeg}" "${cgm_libraw}"; do
  if ! command -v "${executable}" >/dev/null 2>&1 && [[ ! -x "${executable}" ]]; then
    printf 'required media gate executable is unavailable: %s\n' "${executable}" >&2
    exit 1
  fi
done

CGM_TEST_FFMPEG="${cgm_ffmpeg}" CGM_TEST_LIBRAW="${cgm_libraw}" \
  GOTOOLCHAIN=local "${cgm_go}" test ./internal/mediaprocessing -run '^TestExternalMediaMatrix$' -count=1 -v
