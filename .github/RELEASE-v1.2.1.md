# v1.2.1 — all 129 free plugins

## Re-cut — why the version changed again

This PR originally targeted the next patch above `main`'s pre-PR baseline
(one patch past `1.0.0`) for all 129 free-registry entries (see history
below). That target was wrong: measured against `main`, `registry.json` had
41 entries at `1.1.2`, 2 at `1.1.0`, 1 at `1.2.0`, and 1 at `1.1.1`
(`ollama`) — the originally-targeted version would have **downgraded** all
45 of them. A
downgrade breaks inter-plugin `requires` semver ranges
(`internal/plugin/compat.go`) and any installed-version comparison, and the
release tag must equal the registry version because the Cloudflare Worker
resolves `releases/download/v<version>/<name>-<version>.tar.gz` directly
from the `version` field — there is no separate "real" version underneath.

**Every one of the 129 entries now moves to `1.2.1` instead, so no entry
goes backward: `1.2.1` is strictly greater than the highest version anyone
held on `main`** (41 were at `1.1.2`, one — `entitlements` — was at `1.2.0`,
the highest of the lot). `ollama` moves to `1.2.1` with everyone else; the
earlier plan to leave it at `1.1.1` is void — there must be no unreferenced
tarball left dangling on the release.

## Scope change from the original plan (still applies)

This PR originally bumped 8 repackaged plugins (content-progress, cron,
donorbox, maintenance, notifications, notify, search, storage — PRs
#78/#79/#80). **It now covers all 129 free-registry entries.**

The reason: plugins#84 (data audit) found that **70 of the 129 free plugins
had no published tarball for the version `registry.json` stated on `main`**
— 11 had no release tag at all for that version, and 59 had the tag but no
matching `<name>-<version>.tar.gz` asset in it (every `*@1.1.2` entry in
particular — that release is a partial re-release of a different, mostly
`1.0.0`, plugin set, not a superset). `nself plugin install` 404s for all 70
of these today, independent of anything in this PR. Full defect list:
plugins#84.

## What changed in this re-cut

- `registry.json` — `version` set to `1.2.1` for all 129 entries, including
  `ollama`. `releaseTag` set to `v1.2.1` for every entry that carries a
  `releaseTag` field (`shared-utils` does not, and does not gain one —
  `installable:false`, no tarball, no checksum, per below).
- Each plugin's own `free/<name>/plugin.json` — `version` bumped to `1.2.1`
  to match (CI's version-consistency gate requires the two to agree; see
  `.github/workflows/registry-check.yml` "Registry version consistency
  check").
- `scripts/deterministic-tar.sh` now also pins file **modes**
  (`--mode='go-w,a+rX'`), not just mtime/uid/gid/member-order. Measured
  today: identical git content on two machines produced tar streams that
  hashed differently by mode bits alone (`644` on one, `664` on the other —
  the checkout environment's umask was leaking into the artifact). With the
  mode flag added, both machines now produce the identical tar stream.
- `.github/workflows/tarball-checksum-gate.yml` (both the source-tarball
  job and the `checksums.platforms` job added in `0eae661`) no longer
  rebuilds tarballs in CI and diffs them against `registry.json`. That
  comparison can never hold: with byte-identical tar streams as input,
  `gzip -n -9` produces **different output** depending on the gzip/zlib
  build doing the compressing (measured today: Apple gzip 479 vs GNU gzip
  1.12 diverged on real plugin data, 6e9d3966 vs 0283c3b3, for the same
  tar bytes). DEFLATE output is implementation- and version-dependent by
  design — this isn't a bug in the tar recipe, no tar flag fixes it. The
  gate now verifies `registry.json`'s checksums against the **published
  release assets** for the registry's version instead (`gh release
  download`). With no release for that version yet — the normal state of
  this PR right now — it skips with a `::notice::` explaining why and
  passes; once a release exists, a mismatch fails it. The deterministic-tar
  recipe and the mode fix above are unaffected and still required — they
  make a *local* rebuild reproducible across machines, which is what lets a
  contributor sanity-check a tarball before it's uploaded; they were never
  going to make two different gzip implementations agree byte-for-byte.
- `scripts/verify-published-checksums.sh` — rewritten to match the release
  flow this PR depends on: given a tag, it downloads every plugin's
  published release asset (source tarball + any per-platform binaries),
  computes sha256, and either reports mismatches against `registry.json`
  (default) or, with `--write`, writes the published hashes into
  `registry.json`'s flat `checksum`, nested `checksums.sha256`, and
  `checksums.platforms.<platform>` fields. **Not run against a real
  release in this PR** — no `v1.2.1` release exists yet.

## Checksums in this PR are provisional — not rebuilt, not final

**This re-cut did not rebuild any tarball or recompute any checksum.** The
`checksum` / `checksums.sha256` / `checksums.platforms` values currently in
`registry.json` are carried over unchanged from the previous version's
content — they are stale by definition the moment the `version` field next
to them changes, and they must not be read as validated for `1.2.1`. Per
the release flow above, the correct order is: the owner creates a **draft**
`v1.2.1` release with the built tarballs attached, `verify-published-
checksums.sh v1.2.1 --write` then reads the checksums from those published
bytes and writes them into `registry.json`, and only that state gets
merged. Do not treat this PR's current checksum fields as ground truth for
`1.2.1` assets.

## One exception (not touched beyond its version) — `shared-utils`

`shared-utils` has no `checksum` and gains none here. It's
`installable: false` in its own `plugin.json` — an internal Go library
(request-ID tracing middleware, HTTP client propagation) other free
plugins import at build time, not something a user ever
`nself plugin install`s directly. It has no `tarball`/`download_url`/
`releaseTag` field in `registry.json` (not added here, to avoid implying
it's independently distributable) — there is nothing for a checksum to
attest to. Its `version` field is bumped to `1.2.1` for consistency with
its own `plugin.json`, and nothing else.

`ollama` is **no longer** an exception (see re-cut rationale above) — it
takes `1.2.1` and a `releaseTag` of `v1.2.1` like every other entry.

## event-bus asset naming

A previous release misnamed the `event-bus` asset `event-bus-v1.0.0.tar.gz`
(stray `v` inside the filename). Confirmed today: `scripts/build-and-
upload-tarballs.sh` names tarballs `"${plugin_name}-${TAG#v}.tar.gz"`
(strips the `v` from the tag before building the filename), and
`scripts/build-tarballs.sh` uses the same `${version}` (already
`v`-stripped) convention — so a `1.2.1` build of `event-bus` names its
asset `event-bus-1.2.1.tar.gz`, no stray `v`. No occurrence of the slip
remains in either script.

## Local gate

- `git grep -n "1\.0\.1" -- registry.json .github/RELEASE-v1.2.1.md` — empty.
- `jq -r '.plugins[].version' registry.json | sort -u` — exactly `1.2.1`.
- Version-vs-`main` downgrade check across all 129 entries — zero entries
  where the `main` version is greater than `1.2.1`.
- `bash shared/validate-registry.sh` — see PR body for the current error
  count.
- No tarballs were built and no checksums were recomputed this session (see
  "Checksums in this PR are provisional" above) — this is a version-only
  re-cut, not a rebuild.

This is a PUBLIC repo (`nself-org/plugins`) — this account cannot
self-approve, so this PR stays open pending owner review.

## This PR does NOT merge, tag, or release anything

The owner's sequence, once this PR is approved (unchanged in substance from
the release-mechanics note in the builder brief, only the version changes):

```bash
# (a) Owner creates a DRAFT release with the built 1.2.1 tarballs attached,
#     targeting this PR's head commit:
gh release create v1.2.1 -R nself-org/plugins --draft \
  --target <#81-head-sha> \
  -F .github/RELEASE-v1.2.1.md \
  upload-all/*.tar.gz upload-all/*.sha256

# (b) Run verify-published-checksums.sh against the draft's published
#     bytes and push the resulting registry.json to this PR branch:
./scripts/verify-published-checksums.sh v1.2.1 --write
git add registry.json && git commit -m "chore(registry): checksums from published v1.2.1 draft assets"
git push

# (c) Owner merges this PR (this is the version bump landing on main).

# (d) Publish the draft (tag is created on publish, pointing at the merge
#     commit — re-target with --target if the merge produced a different
#     commit than the draft was built against):
gh release edit v1.2.1 -R nself-org/plugins --draft=false

# (e) Purge the Worker's KV cache and spot-verify:
curl -X POST https://plugins.nself.org/api/sync
curl -sI https://plugins.nself.org/plugins/storage/tarball
# expect: HTTP/2 302, location: .../releases/download/v1.2.1/storage-1.2.1.tar.gz
curl -sI https://plugins.nself.org/plugins/access-controls/tarball
# expect: HTTP/2 302
nself plugin install notifications
```

**Do not merge until the owner has reviewed this PR and given the go-ahead.**
