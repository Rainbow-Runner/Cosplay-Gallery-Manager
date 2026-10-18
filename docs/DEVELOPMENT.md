# Source builds

See the root README for the supported CGM frontend and backend build commands.
This public source snapshot omits tests, fixtures, private catalog data and
machine-specific development records. It does not provide the private test suite.
Do not interpret a source build as full business or hardware validation.

Install the pinned web dependencies with `corepack pnpm install --frozen-lockfile`.
Build `ui/web` before building `./cmd/cgm` with the `cgm_web_embed`,
`cgm_galleryepic` and `cgm_moegirl` tags. The legacy Stash build targets are not
the CGM release target. Dependency and license inventories remain in
`THIRD_PARTY_NOTICES.md` and `docs/legal/cgm.spdx.json`.

Never commit local databases, media, credentials, migration packages, screenshots
or deployment logs. After the privacy-related history replacement, use a fresh
clone rather than merging historical development branches into the public tree.
