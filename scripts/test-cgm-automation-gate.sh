#!/usr/bin/env bash
set -euo pipefail

cgm_go="${CGM_GO:-go}"
if ! command -v "${cgm_go}" >/dev/null 2>&1 && [[ ! -x "${cgm_go}" ]]; then
  printf 'required Go toolchain is unavailable: %s\n' "${cgm_go}" >&2
  exit 1
fi

CGM_AUTOMATION_GATE=1 GOMAXPROCS="${CGM_AUTOMATION_GOMAXPROCS:-4}" GOTOOLCHAIN=local "${cgm_go}" test ./internal/persistence/productdb \
  -run '^TestAutomationPerformanceGate$' -count=1 -timeout=10m -v
