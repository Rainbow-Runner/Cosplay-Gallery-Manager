#!/usr/bin/env bash
set -euo pipefail

cgm_docker="${CGM_DOCKER:-docker}"
cgm_toolchain_image="${CGM_CROSS_TOOLCHAIN_IMAGE:-cgm-cross-toolchain:go1.25.12}"
cgm_module_cache="${CGM_GOMODCACHE:-/tmp/cgm-go-mod}"
if ! command -v "${cgm_docker}" >/dev/null 2>&1; then
  printf 'Docker is required for the pinned CGO cross toolchain\n' >&2
  exit 1
fi
if [[ ! -f ui/web/build/index.html ]]; then
  printf 'ui/web/build is missing; run the web production build first\n' >&2
  exit 1
fi
if [[ ! -d "${cgm_module_cache}" ]]; then
  printf 'Go module cache is missing: %s\n' "${cgm_module_cache}" >&2
  exit 1
fi

"${cgm_docker}" build -f docker/cgm/cross.Dockerfile -t "${cgm_toolchain_image}" docker/cgm

cgm_platform_output="$(mktemp -d)"
trap 'rm -rf "${cgm_platform_output}"' EXIT

build_target() {
  local target_os="$1"
  local target_arch="$2"
  local target_cc="$3"
  local suffix="$4"
  printf 'building %s/%s\n' "${target_os}" "${target_arch}"
  "${cgm_docker}" run --rm \
    --user "$(id -u):$(id -g)" \
    --mount "type=bind,src=$(pwd),dst=/source,readonly" \
    --mount "type=bind,src=${cgm_platform_output},dst=/out" \
    --mount "type=bind,src=${cgm_module_cache},dst=/go/pkg/mod" \
    --tmpfs /tmp:rw,exec,size=4g \
    --workdir /source \
    --env CGO_ENABLED=1 --env GOOS="${target_os}" --env GOARCH="${target_arch}" --env CC="${target_cc}" \
    --env GOTOOLCHAIN=local --env GOCACHE=/tmp/go-cache --env GOMODCACHE=/go/pkg/mod \
    "${cgm_toolchain_image}" \
    go build -trimpath -tags "cgm_web_embed cgm_galleryepic" \
    -o "/out/cgm-${target_os}-${target_arch}${suffix}" ./cmd/cgm
}

build_target linux amd64 gcc ""
build_target linux arm64 aarch64-linux-gnu-gcc ""

"${cgm_platform_output}/cgm-linux-amd64" -version
sha256sum "${cgm_platform_output}"/cgm-*
