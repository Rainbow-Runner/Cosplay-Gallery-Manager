# Legal and source correspondence

Cosplay Gallery Manager is licensed under AGPL-3.0-or-later. The root `LICENSE`
contains the complete licence. `THIRD_PARTY_NOTICES.md` records Stash attribution
and the application dependency inventory; `cgm.spdx.json` is the SPDX 2.3 SBOM.

Regenerate both dependency files from the repository root after installing the
pinned Go and web dependencies:

```bash
CGM_GO=/path/to/go scripts/generate-cgm-legal-metadata.mjs
```

A release build must inject the full source commit into
`internal/build.githash`. The public `/about.json` endpoint and `/legal` page then
link to that exact source tree. Development builds without an injected commit are
clearly labelled and link only to the repository root.

The application SBOM describes compiled Go and production web dependencies.
Docker releases must also enable and publish a platform-specific BuildKit
SBOM/attestation so Debian, FFmpeg and dcraw/LibRaw packages are covered.
