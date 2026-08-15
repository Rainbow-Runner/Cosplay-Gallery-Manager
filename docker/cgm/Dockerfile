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
RUN CGO_ENABLED=1 go build -trimpath -tags "cgm_web_embed cgm_galleryepic" \
    -ldflags="-X github.com/stashapp/stash/internal/build.version=${CGM_VERSION} -X github.com/stashapp/stash/internal/build.githash=${CGM_GIT_HASH}" \
    -o /out/cgm ./cmd/cgm

FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl dcraw ffmpeg tzdata \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --system --gid 65532 cgm \
    && useradd --system --uid 65532 --gid 65532 --home-dir /nonexistent --shell /usr/sbin/nologin cgm \
    && install -d -o cgm -g cgm -m 0700 /var/lib/cgm /var/cache/cgm
RUN install -d -o root -g root -m 0755 /etc/cgm
COPY --from=backend --chmod=0555 /out/cgm /usr/local/bin/cgm
COPY --chmod=0444 docker/cgm/cgm.json /etc/cgm/cgm.json
USER 65532:65532
EXPOSE 9999
VOLUME ["/var/lib/cgm", "/var/cache/cgm"]
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["curl", "--fail", "--silent", "--show-error", "http://127.0.0.1:9999/healthz"]
ENTRYPOINT ["/usr/local/bin/cgm"]
CMD ["-config", "/etc/cgm/cgm.json"]
