#!/usr/bin/env bash
set -euo pipefail

cgm_go="${CGM_GO:-go}"
if ! command -v "${cgm_go}" >/dev/null 2>&1 && [[ ! -x "${cgm_go}" ]]; then
  printf 'required Go toolchain is unavailable: %s\n' "${cgm_go}" >&2
  exit 1
fi

CGM_PERFORMANCE_GATE=1 GOMAXPROCS="${CGM_PERF_GOMAXPROCS:-4}" GOTOOLCHAIN=local "${cgm_go}" test ./internal/persistence/productdb \
  -run '^TestPerformanceReleaseGate$' -count=1 -timeout=30m -v
