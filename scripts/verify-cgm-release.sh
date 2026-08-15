#!/usr/bin/env bash
set -euo pipefail

cgm_go="${CGM_GO:-go}"
cgm_version="${CGM_VERSION:-1.5.0-dev}"
cgm_commit="$(git rev-parse HEAD)"

if [[ -n "$(git status --porcelain)" ]]; then
  printf 'release verification requires a clean worktree\n' >&2
  exit 1
fi
for required in LICENSE THIRD_PARTY_NOTICES.md docs/legal/cgm.spdx.json docs/INSTALLATION.md docs/BACKUP_AND_RECOVERY.md; do
  if [[ ! -s "${required}" ]]; then
    printf 'required release file is missing: %s\n' "${required}" >&2
    exit 1
  fi
done

cgm_release_root="$(mktemp -d)"
trap 'rm -rf "${cgm_release_root}"' EXIT

(
  cd ui/web
  corepack pnpm run build
)
GOTOOLCHAIN=local CGO_ENABLED=1 "${cgm_go}" build -trimpath -tags "cgm_web_embed cgm_galleryepic" \
  -ldflags "-X github.com/stashapp/stash/internal/build.version=${cgm_version} -X github.com/stashapp/stash/internal/build.githash=${cgm_commit}" \
  -o "${cgm_release_root}/cgm" ./cmd/cgm

version_output="$("${cgm_release_root}/cgm" -version)"
if [[ "${version_output}" != *"${cgm_version}"* || "${version_output}" != *"${cgm_commit}"* ]]; then
  printf 'binary does not report the requested version and full source commit: %s\n' "${version_output}" >&2
  exit 1
fi

git archive --format=tar.gz --prefix="cosplay-gallery-manager-${cgm_commit}/" \
  -o "${cgm_release_root}/cosplay-gallery-manager-source.tar.gz" "${cgm_commit}"
tar -tzf "${cgm_release_root}/cosplay-gallery-manager-source.tar.gz" \
  "cosplay-gallery-manager-${cgm_commit}/LICENSE" >/dev/null

"${cgm_go}" version -m "${cgm_release_root}/cgm"
sha256sum "${cgm_release_root}/cgm" "${cgm_release_root}/cosplay-gallery-manager-source.tar.gz"
printf 'release source correspondence verified for %s\n' "${cgm_commit}"
