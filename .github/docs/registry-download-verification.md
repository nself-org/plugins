# Verify public plugin downloads

The registry checksum describes the bytes published on the release, including an
asset uploaded while the release was a draft. Never regenerate it from a local
rebuild. Different gzip implementations can produce different archive bytes.

Run `python3 scripts/ci/verify_registry_downloads.py` to download and hash every
installable free plugin in the public registry. It follows each `download_url`,
including the worker's R2 redirect, and checks both checksum aliases when present.
Paid plugins and explicitly noninstallable entries are excluded. Missing metadata,
download failures, mismatches, and an empty free-plugin set fail the check.

`--registry registry.json` checks candidate repository metadata against those same
served download URLs. Registry KV sync runs this after refreshing the worker,
because versionless worker URLs can serve the old release until the sync completes.
A dependent read-only job checks live registry metadata daily and on projection
changes. It runs even when sync fails, while waiting for sync to finish to avoid a
race against the cache refresh.

The release workflow already downloads published assets to generate release data
with `scripts/verify-published-checksums.sh`. If only repository checksums are wrong,
the owner can run `bash scripts/verify-published-checksums.sh --write <tag>` followed
by `bash scripts/plugin-counts.sh` and review the diff. A changed signed checksum
requires the normal signing flow. If GitHub bytes match but R2 bytes differ, report
the object URL and both hashes to the release owner. Repairing R2, replacing release
assets, or refreshing production caches requires a separate authorized operation.

Offline tests: `python3 -m unittest discover -s scripts/ci/tests -p 'test_verify_registry_downloads.py'`.
