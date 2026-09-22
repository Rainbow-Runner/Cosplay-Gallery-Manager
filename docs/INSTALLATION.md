# Cosplay Gallery Manager installation

This document covers the first-version native and Docker layouts. CGM is a
single-owner, local SQLite application. It has no network scraper, updater,
telemetry, plugin execution or application API for deleting source media.

## Supported targets

- Linux amd64 and arm64.
- Docker linux/amd64 and linux/arm64.

Windows and macOS native packages, 32-bit systems, GPU processing and mobile
native applications are deferred and not supported in the first version.

## Native installation

1. Obtain the executable, `LICENSE`, `THIRD_PARTY_NOTICES.md`, SPDX SBOM and
   source archive from the same release. Verify the published checksums.
2. Install FFmpeg/FFprobe and a LibRaw-compatible `dcraw` executable if video
   or RAW processing is required.
3. Create private writable directories for the database, generated cache,
   managed Coser metadata and backups. The media directory itself may and
   preferably should be read-only to the CGM process.
4. Create `cgm.json` and start `cgm -config /absolute/path/cgm.json`.
5. Open the configured loopback URL and complete the five-step Setup. Setup
   never starts the first media scan automatically.

Linux example:

```json
{
  "listen": "127.0.0.1:9999",
  "database_path": "/var/lib/cgm/product.sqlite",
  "cache_path": "/var/cache/cgm",
  "ffmpeg_path": "/usr/bin/ffmpeg",
  "ffprobe_path": "/usr/bin/ffprobe",
  "libraw_path": "/usr/bin/dcraw",
  "worker_count": 2,
  "log_level": "INFO",
  "metadata_scraping_enabled": false,
  "entity_metadata_scraping_enabled": false
}
```

`ffprobe_path` is optional when a matching executable is installed beside
`ffmpeg_path`; CGM resolves that sibling first and reports both dependency
versions in Manage → Settings. Missing video tools do not prevent image and
RAW workflows from starting, but video probe/poster/proxy jobs report stable
diagnostic codes until the dependencies are available.

`listen` must contain an explicit host and port. `worker_count` must be from 1
through 8. `log_level` accepts `DEBUG`, `INFO`, `WARN`, or `ERROR`; use
`DEBUG` only while diagnosing a local development instance. Request logs use
technical request IDs and endpoint categories and do not include query
strings, GraphQL variables, media paths, or business metadata. A native
first-time Setup reached through a literal loopback bind does not require a
ticket.

`metadata_scraping_enabled` and `entity_metadata_scraping_enabled` both default
to `false`. The first controls Coser profile imports; the second controls Work
and Character name imports. When enabled, only an authenticated owner's
explicit action in the corresponding editor may contact a provider compiled
into that build. Browse, source scans, startup, and scheduled jobs remain
offline. Removing either provider adapter requires no database migration and
does not disable ordinary entity editing.

## Docker Compose

The supplied image runs as UID/GID 65532, embeds the web UI, includes FFmpeg
and dcraw, uses a read-only root filesystem and stores mutable state in named
volumes. The media mount is writable only so explicit Manifest sidecar Push can
work; CGM never rewrites source photos, videos or archives.

```bash
export CGM_MEDIA_ROOT=/absolute/path/to/cosplay-media
docker compose -f docker/cgm/compose.yml build
docker compose -f docker/cgm/compose.yml up -d
```

Open `http://127.0.0.1:9999/setup` on the Docker host. The supplied Compose
binds the port only to host loopback and explicitly enables local first-time
Setup, so no terminal ticket is required. Do not publish this Compose service
to a public or LAN address before Setup. If using a custom remote deployment,
omit `CGM_LOCAL_DOCKER_SETUP=1` and generate a one-time ticket with
`docker compose run --rm --no-deps cgm -setup-ticket`; enter it in the Setup
page. Never put the ticket in Compose files, URLs, logs or shell history.

Docker Setup fixes Coser metadata at `/var/lib/cgm/cosers` and backups at
`/var/lib/cgm/backups` inside the `cgm-state` volume, checking both directories
for actual write access before completion. Setup also requires explicit mounts
at `/var/lib/cgm`, `/var/cache/cgm`, `/media`, and `/transfer`; the image does
not create anonymous data volumes when these are missing. The database also lives in this
volume (`/var/lib/cgm/product.sqlite`); generated cache lives separately in
`cgm-cache` (`/var/cache/cgm`). These are container paths, not host paths.
The host media directory specified by `CGM_MEDIA_ROOT` appears at `/media`;
create media libraries only at `/media` or existing subdirectories. The host
must permit container UID/GID 65532 to read media and write Manifest sidecars
where writeback is enabled. A read-only media mount remains possible when
writeback is not needed, but it blocks Push. A custom Compose file must
provide equivalent persistent mounts; Setup does not create a media library
or start scanning.

Place portable migration `.zip` packages in `docker/cgm/transfer` on the host
(`./transfer` relative to the Compose file). The container sees these files in
`/transfer` read-only. Refresh the migration workbench, select a package, and
run its full pre-import check before importing or preparing a merge. Export
destinations remain separate server/container paths and must be writable; for
example, use an existing writable directory under `/var/lib/cgm`. A custom
Compose deployment must mount its own transfer directory at `/transfer`.

If the owner password is forgotten, run the following on the machine that
holds the state volume and enter the printed token in Login → Forgot password:

```bash
docker compose -f docker/cgm/compose.yml exec cgm /usr/local/bin/cgm -config /etc/cgm/cgm.json -recovery-token
```

The token is single-use and expires after 10 minutes. Issuing a new token
invalidates the previous one. A successful reset revokes all sessions and
disables trusted mode. For native installations, run `cgm -config
/absolute/path/cgm.json -recovery-token` locally. Do not expose the token in
shell history, logs, support requests or URLs. The recovery table is added by
product database schema v16; upgrades from v15 create a mandatory pre-migration
SQLite snapshot before changing the schema. Portable package format v2 and
Gallery manifests are unchanged. Existing full backups made by schema v15
remain archival recovery material, but the current in-app restore accepts
only backups matching its current database schema; restore a v15 backup using
the corresponding v15 application, then upgrade again.

Production image builds must inject the release identity and publish a
platform-specific image SBOM:

```bash
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --build-arg CGM_VERSION=1.0.0 \
  --build-arg CGM_GIT_HASH="$(git rev-parse HEAD)" \
  --sbom=true --provenance=true \
  -f docker/cgm/Dockerfile .
```

## Reverse proxy

CGM has no built-in TLS. Keep it bound to loopback or a private container
network and terminate HTTPS at a maintained reverse proxy. Preserve the
original `Host`, proxy Web/API requests to one CGM instance, set conservative
request-body limits that still permit the documented Coser image upload, and
do not cache `/graphql`, `/session/*`, `/maintenance/*`, `/about.json` or
authenticated `/resource/*` responses.

The current server does not interpret forwarded-protocol headers when setting
cookies. Treat direct HTTP exposure, public Internet exposure and multiple
active CGM replicas as unsupported. Enforce HTTPS and HSTS at the proxy.

## Media libraries and permissions

- Add only absolute library roots.
- Overlapping roots use the most-specific enabled root; disabled child roots
  are boundaries.
- CGM reads source files and may write an explicit Gallery Manifest only when
  the source is writable and the owner requests Push.
- CGM never deletes, moves or rewrites user media. Keep independent filesystem
  backups; the CGM backup package does not contain media or Gallery sidecars.
- On network mounts, make mount availability an operating-system service
  dependency. A missing source changes availability; it does not silently
  delete Item identity.

## Upgrade

1. Create and verify a full CGM backup and a separate media/filesystem backup.
2. Stop the old process. Do not run two CGM versions against one SQLite file.
3. Replace the executable or image and keep the database, cache, Coser metadata
   and backup volumes.
4. Start the new version and confirm `/readyz`, login, Browse and Operations.
5. If a future schema upgrade is required, CGM must create its mandatory
   pre-upgrade snapshot before migration. Never point CGM at a Stash or unknown
   non-empty database; the product identity guard rejects it.

The About & Legal page shows the running version, source commit when injected,
AGPL licence, warranty statement and corresponding source link.
