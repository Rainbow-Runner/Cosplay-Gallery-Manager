# Public source publication

The public branches contain buildable, privacy-reviewed source snapshots, not
the full internal development history. Both public branches use the same clean
history. Source releases preserve the AGPL licence, Stash attribution, build
inputs, dependency lockfiles and third-party notices.

Excluded: tests, test data and snapshots, benchmarks and business audits, media
catalogs, private development notes, machine-specific deployment records,
screenshots, databases, credentials and local environment files. Public build
configuration does not depend on the omitted test suite. Test dependencies may
remain pinned in lockfiles; they are not private test source or business data.

Do not push or merge an unsanitized branch into this repository. Review both
file contents and commit ancestry before publication. Re-clone after the
history replacement; do not reintroduce old commits with a pull or merge.

Replacing Git references does not erase existing third-party clones, forks,
GitHub cached commit views or pull-request references. Cached historical data
may require a separate GitHub Support removal request. This publication does
not claim that externally copied data can be recalled.

Docker images identify their privacy-reviewed source snapshot in revision
metadata. Registry publication and GitHub branch updates can happen separately.
Images built with the bundled-source release workflow include the exact clean
source archive at `/usr/share/cgm/source.tar.gz`; it contains no private history
or tests. To retrieve it without starting the application:

```sh
docker create --name cgm-source IMAGE
docker cp cgm-source:/usr/share/cgm/source.tar.gz ./cgm-source.tar.gz
docker rm cgm-source
```

Replace IMAGE with the chosen immutable image tag or digest. Keep the source
archive, source revision and image digest together for reproducibility. The
archive is also available when the corresponding GitHub update is pending.
