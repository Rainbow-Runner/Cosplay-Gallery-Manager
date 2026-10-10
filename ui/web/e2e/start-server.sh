#!/usr/bin/env bash
set -euo pipefail

e2e_runtime="$(pwd)/e2e/.runtime"
cgm_go="${CGM_GO:-/tmp/cgm-go1.25.12/bin/go}"
if [[ ! -x "${cgm_go}" ]]; then
  cgm_go="$(command -v go)"
fi

rm -rf "${e2e_runtime}"
mkdir -p "${e2e_runtime}/cache" "${e2e_runtime}/cosers" "${e2e_runtime}/backups"

GOTOOLCHAIN=local GOCACHE=/tmp/cgm-go-cache GOMODCACHE=/tmp/cgm-go-mod \
  "${cgm_go}" run ./e2e/fixturegen "${e2e_runtime}/library/playwright-gallery"
GOTOOLCHAIN=local GOCACHE=/tmp/cgm-go-cache GOMODCACHE=/tmp/cgm-go-mod \
  "${cgm_go}" build -tags cgm_web_embed -o "${e2e_runtime}/cgm" ../../cmd/cgm

printf '%s\n' \
  '{' \
  '  "listen": "127.0.0.1:3210",' \
  "  \"database_path\": \"${e2e_runtime}/product.sqlite\"," \
  "  \"cache_path\": \"${e2e_runtime}/cache\"," \
  '  "worker_count": 1' \
  '}' > "${e2e_runtime}/cgm.json"

exec "${e2e_runtime}/cgm" -config "${e2e_runtime}/cgm.json"
