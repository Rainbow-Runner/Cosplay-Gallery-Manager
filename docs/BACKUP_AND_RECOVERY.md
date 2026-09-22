# Backup and recovery

CGM backups protect the product database and managed Coser metadata. They do
not contain user media, Gallery sidecar Manifests, generated cache or logs.
Keep a separate filesystem backup for those items.

The first-version backup format is not encrypted. Protect the database, backup
root and exported packages with operating-system permissions and disk or
volume encryption such as BitLocker or LUKS.

## Backup types

- Daily snapshot: a consistent SQLite online backup, enabled by default, with
  a default retention of seven.
- Full package: a ZIP containing a consistent product database, managed Coser
  metadata, required startup configuration and the product/database/Manifest/
  media-profile version manifest.
- Safety package: a full package created automatically immediately before a
  restore replaces live state.

Create a full package from Manage → Operations. The CLI equivalent requires an
interactive terminal and owner password:

```bash
cgm -config /absolute/path/cgm.json -create-backup
```

The package is written beneath the configured backup root with exclusive
creation; CGM never silently overwrites an existing backup. Copy completed
packages to independent storage and periodically exercise a restore on a
separate installation.

## Restore in the web interface

1. Sign in and open Manage → Operations.
2. Select a full backup and type the exact confirmation `RESTORE`.
3. CGM verifies the recorded SHA-256, product identity, schema/version manifest
   and a strict safe-ZIP policy before replacement.
4. CGM creates a safety package, enters maintenance, stages the database and
   Coser metadata, then swaps them. A failed swap automatically attempts to
   restore the previous live state.
5. A successful restore revokes all sessions, cancels old executable jobs,
   clears scheduler leases and suspends automatic schedules.
6. Sign in again on the maintenance page. For every restored library, select a
   real absolute root on this machine or disable it. Type the exact
   confirmation `MAP`.
7. Validate writable application roots, configured media executables and every
   enabled media root, then explicitly resume. CGM does not automatically scan
   or Pull/Push a Manifest during this process.
8. Start a manual discovery/rescan, review issues and only then re-enable
   schedules as desired.

## Restore with the CLI

All commands require an interactive terminal and owner reauthentication:

```bash
cgm -config /absolute/path/cgm.json -restore-backup BACKUP_UUID
cgm -config /absolute/path/cgm.json -map-restored-paths
cgm -config /absolute/path/cgm.json -resume-maintenance
```

The first command asks for the exact word `RESTORE`. Path mapping prompts once
for every restored library; an empty new path disables that library, and the
final confirmation is `MAP`. Resume succeeds only after the same environmental
checks used by the web workflow.

For Docker, run the CLI against the same stopped service volumes. Stop the
server first so only one process performs the maintenance operation:

```bash
docker compose -f docker/cgm/compose.yml stop cgm
docker compose -f docker/cgm/compose.yml run --rm --no-deps cgm -restore-backup BACKUP_UUID
docker compose -f docker/cgm/compose.yml run --rm --no-deps cgm -map-restored-paths
docker compose -f docker/cgm/compose.yml run --rm --no-deps cgm -resume-maintenance
docker compose -f docker/cgm/compose.yml up -d
```

## Failure handling

- Do not manually edit or repack a backup ZIP.
- A validation failure before replacement leaves the live database unchanged.
- A replacement failure records a technical error code and attempts an
  automatic rollback. Preserve the live database, WAL/SHM files, backup root
  and logs before further intervention.
- If maintenance remains active, `/login`, `/maintenance`, `/legal`,
  `/about.json`, `/healthz` and `/readyz` remain reachable; business routes are
  intentionally unavailable.
- Never bypass pending path mapping by editing SQLite. Map or disable every
  restored library through the supported workflow.
- Search indexes, Tag closure, recommendation caches and counts are
  reconstructible. Source media and Gallery Manifests are authoritative files
  outside the backup package and must be restored separately.

## Recovery drill checklist

Record the package UUID and hash, restore onto a clean machine, map at least one
library to a different absolute path, disable one unavailable library, confirm
login/session revocation, verify Coser images, run a manual scan, inspect a
Gallery Manifest conflict without overwriting it, browse media, create a new
full backup, and retain the drill result with the release/platform details.
