# v1.0.1 — repackaged free plugins

## What changed

Eight free plugins were repackaged in PRs #78/#79/#80 (self-contained build
contexts, `/health` routes on the seven that run an HTTP server, stale
license gates removed) but `registry.json` still pointed at the v1.0.0
tarballs on the `v1.0.0` GitHub Release. `nself plugin install <name>` for
any of these eight therefore still installed the pre-#78/#79/#80 code.

This release bumps those eight plugins' `version` to `1.0.1` (in both
`registry.json` and each plugin's own `plugin.json`, which CI's version-
consistency gate requires to match) and republishes their tarballs under
tag `v1.0.1`, so the registry worker's tarball redirect
(`.workers/plugins-registry/src/index.ts` `handlePluginTarball`, which
builds the GitHub Releases URL from `plugin.version`) starts serving the
repackaged code.

**Plugins bumped 1.0.0 → 1.0.1** (all free, `requires_license: false`):
content-progress, cron, donorbox, maintenance, notifications, notify,
search, storage.

Nothing else in the registry changed — the other 121 plugins keep their
existing versions and tarball URLs.

## Also recorded on the registry entries

- `checksum` (flat field) — the field `internal/plugin/registry_parse.go`'s
  `pluginEntry` actually deserializes (`json:"checksum"`) and
  `verifyChecksum` in `internal/plugin/installer_locked.go` checks against.
  Populated with the tarball's real SHA-256 for all eight.
- `checksums.sha256` and `releaseTag` — the fields `registry-schema.json`
  documents and `build-and-upload-tarballs.sh` writes. Populated too, for
  schema/tooling parity, even though nothing in this repo or the deployed
  worker currently reads them (see Known gap below).

None of the eight plugins have `"status": "stable"` set, so
`verifyChecksum` does not hard-fail installs that lack a checksum — but a
correct one is now present regardless.

## Known gap (pre-existing, not introduced by this PR)

`registry-schema.json` and `build-and-upload-tarballs.sh` write checksums to
the nested `checksums.sha256` field. The CLI's registry parser
(`internal/plugin/registry_parse.go`) only ever reads the flat `checksum`
field — `checksums.sha256` is never consulted. Any future release built by
running `build-and-upload-tarballs.sh` as-is will silently populate a field
the installer ignores. Worth a follow-up ticket to fix the script (or add
the flat alias) so this doesn't bite a `status: stable` plugin later.

## `scripts/build-tarballs.sh` behavior

`scripts/build-tarballs.sh 1.0.1` builds a tarball for **every** plugin
under `free/` (129 of them) at the single VERSION argument you pass, tagging
even untouched plugins as `<name>-1.0.1.tar.gz` — it does not read each
plugin's own `registry.json` version. This release keeps only the 8
relevant tarballs; the other 121 mislabeled ones were discarded, not
uploaded, and never committed (dist/ is gitignored).

## `maintenance` has no `/health` route

`maintenance` is a Cobra CLI tool (`cmd/main.go`, disk-cleanup/scheduler
subcommands) with no HTTP server anywhere in its source — it never had a
`/health` route and #78/#79 didn't add one, correctly, since there is no
HTTP surface to health-check. This is not a gap in this release.

## Verify commands

```bash
# Tarball redirect resolves to the new release
curl -sI https://plugins.nself.org/plugins/storage/tarball
# expect: HTTP/2 302, location: .../releases/download/v1.0.1/storage-1.0.1.tar.gz

# Fresh install reaches a healthy container
nself plugin install notifications
curl -sf http://localhost:<notifications-port>/health
```
