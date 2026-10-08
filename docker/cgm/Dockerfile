# syntax=docker/dockerfile:1.7

FROM --platform=$BUILDPLATFORM node:24-bookworm-slim AS web
RUN corepack enable
WORKDIR /source/ui/web
COPY ui/web/package.json ui/web/pnpm-lock.yaml ./
RUN corepack pnpm install --frozen-lockfile
COPY ui/web/ ./
RUN corepack pnpm run build

FROM --platform=$TARGETPLATFORM golang:1.25.12-bookworm AS backend
ARG CGM_VERSION=1.5.0-dev
ARG CGM_GIT_HASH
WORKDIR /source
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /source/ui/web/build /source/ui/web/build
RUN CGO_ENABLED=1 go build -trimpath -tags "cgm_web_embed cgm_galleryepic cgm_moegirl" \
    -ldflags="-X github.com/stashapp/stash/internal/build.version=${CGM_VERSION} -X github.com/stashapp/stash/internal/build.githash=${CGM_GIT_HASH}" \
    -o /out/cgm ./cmd/cgm

FROM debian:bookworm-slim
COPY docker/cgm/intel.sources /etc/apt/sources.list.d/cgm-intel.sources
RUN set -eu; apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl dcraw ffmpeg tzdata vainfo libva-drm2 \
    && if [ "$(dpkg --print-architecture)" = amd64 ]; then \
         apt-get install -y --no-install-recommends intel-media-va-driver-non-free; \
         test -s /usr/lib/x86_64-linux-gnu/dri/iHD_drv_video.so; \
         ldd /usr/lib/x86_64-linux-gnu/dri/iHD_drv_video.so | tee /tmp/cgm-driver-dependencies; \
         ! grep -q 'not found' /tmp/cgm-driver-dependencies; \
       fi \
    && ffmpeg -hide_banner -encoders 2>/dev/null | grep -q h264_vaapi \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --system --gid 65532 cgm \
    && useradd --system --uid 65532 --gid 65532 --home-dir /nonexistent --shell /usr/sbin/nologin cgm \
    && install -d -o cgm -g cgm -m 0700 /var/lib/cgm /var/cache/cgm
RUN install -d -o root -g root -m 0755 /etc/cgm
COPY --from=backend --chmod=0555 /out/cgm /usr/local/bin/cgm
COPY --chmod=0444 docker/cgm/cgm.json /etc/cgm/cgm.json
USER 65532:65532
EXPOSE 9999
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["curl", "--fail", "--silent", "--show-error", "http://127.0.0.1:9999/healthz"]
ENTRYPOINT ["/usr/local/bin/cgm"]
CMD ["-config", "/etc/cgm/cgm.json"]
