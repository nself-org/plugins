# v1.0.1 — all 129 free plugins

## Scope change from the original plan

This PR originally bumped 8 repackaged plugins (content-progress, cron,
donorbox, maintenance, notifications, notify, search, storage — PRs
#78/#79/#80) to `1.0.1`. **It now covers all 129 free-registry entries.**

The reason: plugins#84 (data audit, still open) found that **70 of the 129
free plugins have no published tarball for the version `registry.json`
currently states** — 11 have no release tag at all for that version, and 59
have the tag but no matching `<name>-<version>.tar.gz` asset in it (every
`*@1.1.2` entry in particular — that release is a partial re-release of a
different, mostly-1.0.0 plugin set, not a superset). `nself plugin install`
404s for all 70 of these today, independent of anything this PR changes.
Bumping only the 8 already-repackaged plugins would have shipped a v1.0.1
release that still left the other 121 registry entries pointing at broken
or unverified download links. Full defect list: plugins#84.

**v1.0.1 fixes this by aligning every registry entry's version to `1.0.1`
and having the build pipeline produce a real, matching tarball for all of
them**, so `download_url` → `tarball` → the release asset is a working
chain for the whole free catalog, not just 8 plugins.

## What changed

- `registry.json` — `version` set to `1.0.1` for 128 of 129 entries (see
  exceptions below), `tarball` URL rewritten to
  `.../releases/download/v1.0.1/<name>-1.0.1.tar.gz`, `releaseTag` set to
  `v1.0.1`.
- Each plugin's own `free/<name>/plugin.json` — `version` bumped to `1.0.1`
  to match (CI's version-consistency gate requires the two to agree; see
  `.github/workflows/registry-check.yml` "Registry version consistency
  check").
- `scripts/build-tarballs.sh 1.0.1` built a tarball for all 129 plugins
  under `free/`. All 129 tarballs + `.sha256` files are kept for the
  release upload (`dist/` stays gitignored — not committed to git).
- `checksum` (flat field — what `internal/plugin/registry_parse.go`
  actually deserializes and `installer_locked.go`'s `verifyChecksum`
  checks) and `checksums.sha256` (nested — what `registry-schema.json`
  documents and `build-and-upload-tarballs.sh` writes) written for every
  entry from the tarball this pipeline actually built this session — not
  trust-on-first-use against an already-published asset. **Checksum
  coverage: 128/129**, up from 1/129 before this PR.

## Two exceptions (not bumped to 1.0.1) — and why

- **`ollama`** stays at `1.1.1`. It already has a real, working,
  currently-served release (`v1.1.1`, verified: `ollama-1.1.1.tar.gz` is a
  genuine asset on that tag) — forcing it back to `1.0.1` would have been a
  regression, shipping older code under a newer-looking version number.
  Its `checksum`/`checksums.sha256`/`releaseTag` were still written this
  session: the real `v1.1.1` asset was downloaded from the GitHub release
  and hashed (`affe917f...9d52f`), matching the already-published
  `.sha256` sidecar file byte-for-byte. This replaces the previous empty
  `""` placeholder with a verified value, without touching its version.
- **`shared-utils`** has no `checksum` (129th entry, the only one without
  one). It's `installable: false` in its own `plugin.json` — an internal Go
  library (request-ID tracing middleware, HTTP client propagation) other
  free plugins import at build time, not something a user ever
  `nself plugin install`s directly. It has no `tarball`/`download_url`
  field in `registry.json` at all (not added here, to avoid implying it's
  independently distributable) — there is nothing for a checksum to
  attest to. Its `version` was still bumped to `1.0.1` for consistency
  with its own `plugin.json`, and `scripts/build-tarballs.sh` (which walks
  every `free/*/plugin.json` unconditionally) still built a
  `shared-utils-1.0.1.tar.gz` — kept in the upload set for completeness,
  simply not referenced by any registry field.

## plugins#84 TOFU baselines — superseded

plugins#84 populated the flat `checksum` field for 51 previously-empty
entries (plus fixing the pre-existing `ollama` `""` placeholder) as
trust-on-first-use baselines against each entry's **then-current**
published version. Every one of those 51 entries' `version` has now moved
to `1.0.1` in this PR, so plugins#84's baseline checksums — computed
against the old version's asset — no longer match the `version` field they
would be checked against and are superseded by the pipeline-generated
values here. plugins#84 itself is unaffected/still valid as a historical
record of the TOFU methodology; its checksums for the moved entries should
not be merged over this PR's.

## Known gap (pre-existing, not introduced by this PR)

`registry-schema.json` and `build-and-upload-tarballs.sh` write checksums
to the nested `checksums.sha256` field. The CLI's registry parser
(`internal/plugin/registry_parse.go`) only ever reads the flat `checksum`
field — `checksums.sha256` is never consulted. Both are populated here for
schema/tooling parity, but only the flat field is load-bearing for install
verification today.

## `maintenance` has no `/health` route

`maintenance` is a Cobra CLI tool (`cmd/main.go`, disk-cleanup/scheduler
subcommands) with no HTTP server anywhere in its source — it never had a
`/health` route and #78/#79 didn't add one, correctly, since there is no
HTTP surface to health-check. Not a gap in this release.

## Local gate (self-hosted CI backlog — per P6 crunch brief §8, not watched)

- `python3 -c "import json; json.load(open('registry.json'))"` — valid.
- Version-consistency check (mirrors `.github/workflows/registry-check.yml`
  "Registry version consistency check", reproduced by hand across all 129
  `free/*/plugin.json` files): 0 mismatches.
- `bash shared/validate-registry.sh` — 0 errors, 1 pre-existing unrelated
  warning (alphabetical sort order — same before and after this change).
- `bash shared/validate-registry.sh --strict` — 0 errors, 1 additional
  informational warning (strict legacy schema mode not re-enabled in v2 —
  script-level notice, not a registry defect).
- 5 randomly sampled built tarballs (`job-queue`, `nself-eval-gate`,
  `devices`, `release`, `vpn`) extracted cleanly to `free/<name>/` with a
  `plugin.json` at `version: 1.0.1` inside.
- `pnpm run ci:local` — passes (no tsconfig in this repo, gate no-ops by
  design, same as before).

This is a PUBLIC repo (`nself-org/plugins`) — this account cannot
self-approve, so this PR stays open per the P6 crunch brief. GitHub-hosted
Actions will run automatically; not watched here per brief §8.

## This PR does NOT merge, tag, or release anything

Per the P6 crunch brief this session may not create tags, run
`gh release create`, or merge. The owner's remaining action, once this PR
is approved, is exactly these three commands:

```bash
# 1. Merge this PR (owner reviews + merges), then tag the merge commit:
git -C <plugins-checkout> fetch origin main
git -C <plugins-checkout> tag v1.0.1 <merge-sha>
git -C <plugins-checkout> push origin v1.0.1

# 2. Create the release and attach all 129 tarballs + their .sha256 files
#    (built locally in this session, NOT committed to git — dist/ stays
#    gitignored). ollama is intentionally NOT among these 129 — it ships
#    under its own existing v1.1.1 release, untouched by this PR:
gh release create v1.0.1 -R nself-org/plugins \
  /private/tmp/claude-501/-Volumes-UG-Sites-nself/fc66fc90-9fbd-441a-8372-5626c5d07a60/scratchpad/plugins-v1.0.1-dist/upload-all/*.tar.gz \
  /private/tmp/claude-501/-Volumes-UG-Sites-nself/fc66fc90-9fbd-441a-8372-5626c5d07a60/scratchpad/plugins-v1.0.1-dist/upload-all/*.tar.gz.sha256 \
  -F .github/RELEASE-v1.0.1.md

# 3. Verify (tarball redirect + a previously-404ing plugin + a fresh install):
curl -sI https://plugins.nself.org/plugins/storage/tarball
# expect: HTTP/2 302, location: .../releases/download/v1.0.1/storage-1.0.1.tar.gz

curl -sI https://plugins.nself.org/plugins/access-controls/tarball
# expect: HTTP/2 302 (was 404 before this release — one of plugins#84's 70)

nself plugin install notifications
curl -sf http://localhost:<notifications-port>/health
```

**Do not merge until the owner has reviewed this PR and given the go-ahead.**
