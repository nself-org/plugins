# Releasing

## Binary targets

The free-plugin release (`.github/workflows/release-tarballs.yml`) compiles a command binary for every
plugin that ships one, for five platforms. Which binaries, and which commands they carry, is not read
from `plugin.json` by the workflow. `scripts/release/cli-targets.sh DIR` prints them, one
`binary<TAB>command` line each, from the cli tool `manifestv2migrate -targets`.

- The tool normalizes a v1 or v2 `plugin.json` and applies the installer's own rule
  (`manifestv2.CLITargets`, the same function `nself add` uses to decide which binaries to link), so
  the release builds exactly what the installer will look for.
- `scripts/release/cli-targets-gate.sh` runs over every free plugin first and decides what may ship. It blocks a plugin
  whose commands include a core verb or top-level command of the pinned cli (read from that commit's
  `.github/command-registry.json`), a name outside `^[a-z][a-z0-9-]*$`, a binary another plugin also yields, or a
  manifest the normalizer refuses. Each blocked plugin needs an unexpired entry (rule `cli-targets`, owner_ref naming
  the fixing Ticket, expiry within the v1.5.0 cap) in `scripts/conformance/exceptions.yaml`, else the run fails.
  Blocked plugins ship no binary and are listed in the job summary.
- A plugin with no command prints nothing and exits 0. A manifest the tool refuses exits 1; the gate turns that into a block that needs an exception.
- The tool comes from the cli commit pinned in `scripts/cli-tools.version`. The workflow builds it once
  (Go, from that commit's vendor directory) and shares it with the parallel plugin builds. Changing the
  pin is the way to pick up a new rule.
- Fixtures: `tests/fixtures/manifest-v1-cli/` and its v2 twin `tests/fixtures/manifest-v2-cli/` must print
  identical targets.

## Signed, attested release chain

A release tag (`v*`) or a manual run of `release-tarballs.yml` with a tag input runs four jobs. Each holds only what it needs, and
the chain uses the workflow's own `GITHUB_TOKEN` plus scoped repository secrets: no personal access token and no cross-repository trigger.

| Job | Permissions | Secrets | Does |
|---|---|---|---|
| `build` | contents read | none | deterministic source and per-platform tarballs (`scripts/deterministic-tar.sh`) |
| `publish` | contents write, id-token write, attestations write | Cloudflare R2 (R2 step only) | uploads missing tarballs to the Release (never `--clobber`), re-downloads the published bytes, writes the `.sha256` files from them, mirrors those bytes to R2 and reads each object back, attests build provenance for the tarballs this run built, records the published sha256 list (also in the job summary) |
| `sign` | contents read | free-tier signing key (signing step only) | bound to the `release-signing` Environment; downloads what the Release serves, refuses it unless it equals the list `publish` recorded, signs the source tarballs; runs the tag's own commit of `sign.sh` |
| `release-data` | contents write, pull-requests write | none | writes `releases.json` from the published bytes and the signatures, opens the release-data PR |

Checksums and signatures always describe the bytes on the Release, never a rebuild. Gzip output differs between machines, so an
asset that is already on the Release stays the artifact of record and everything downstream follows it.

### Signature format (contract `plugins.release-signature` v1, hq PLUG D7)

- Message: `nself-plugin-v1\n<slug>\n<version>\n<sha256 hex of the source tarball>\n`, signed with raw Ed25519
  (`openssl pkeyutl -sign -rawin`).
- `release_signature` in `releases.json` and the projections: `{"key_id": "plugins-<16 hex>", "alg": "ed25519", "sig": "<base64>"}`.
  `sig` is the 64-byte signature in standard padded base64 (the text `sdk/go/signing` `EncodeSig` produces); `verify-signatures.sh` accepts only that canonical text. `key_id` is derived,
  never chosen: `plugins-` plus the first 16 hex of sha256 over the raw 32-byte public key. `sign.sh` refuses a key id that does not match its key.
- Only the source tarball `<slug>-<version>.tar.gz` is signed. The per-platform binary tarballs are covered by `platform_checksums` (in the unsigned
  `releases.json`) and by the provenance attestation; verify them with `gh attestation verify <file> --repo nself-org/plugins`. Only bytes this run built
  and the Release still serves unchanged are attested: a tarball uploaded earlier (a draft upload, an older run) is published and checksummed but carries no
  attestation, and the `publish` job summary says which.
  `verify-signatures.sh` counts source tarballs (checksum + signature) and binary tarballs (checksum only) separately.
- A source tarball is signed only when its plugin's manifest `version` equals the tag's version; the signed set is exactly the set of rows
  `release-data` writes. Both scripts accept Ed25519 keys only and refuse anything else.
- Verification order is checksum, then signature, both before extraction.

### Scripts (`scripts/release/`)

| Script | Use |
|---|---|
| `sign.sh SLUG VERSION TARBALL` | prints the `release_signature` JSON. Key from `NSELF_RELEASE_SIGNING_KEY_FREE` (PKCS8 PEM) and `NSELF_RELEASE_SIGNING_KEY_FREE_ID`, environment only; written to a 0600 temp file removed on exit |
| `verify-signatures.sh --pub FILE DIR` | checks every `*.tar.gz` in DIR against `DIR/releases.json` and exactly the public key FILE (PEM SPKI). Failures on stderr: `checksum mismatch`, `missing release data`, `missing tarball`, `unsigned`, `signature mismatch`. No key discovery |
| `rehearse.sh --tag T --no-upload --out DIR` | builds all source tarballs and signs them with a throwaway key; writes `DIR/releases.json` and `DIR/rehearsal-test.pub`. Two runs of one tag give byte-identical tarballs |
| `keys/` | committed release public keys (`plugins-release-free-<id>.pub`). Empty until P7-PLUG-69 commits the production key, which must happen before the first real tag run |

OpenSSL 3 is required (`OPENSSL=/path/to/openssl` to pick one); macOS system LibreSSL cannot sign or verify Ed25519 with `-rawin`.
GNU tar is required for deterministic tarballs (`brew install gnu-tar`).

### Rehearsal (no key, no release, no network)

```bash
bash scripts/release/rehearse.sh --tag v0.0.0-rehearsal --no-upload --out "$TMPDIR/rehearsal"
bash scripts/release/verify-signatures.sh --pub "$TMPDIR/rehearsal/rehearsal-test.pub" "$TMPDIR/rehearsal"
```

The same directory fails against any other key (`signature mismatch`, including every committed key in `keys/`) and a one-byte change in any
tarball fails with `checksum mismatch`. A check that only compared sha256 to `releases.json` would pass for any key; this one does not.
The Ticket P7-PLUG-21 verification lines are the gate for these scripts; no workflow runs them yet.

### Key custody and rotation

- The free-tier key is an Actions secret of the `release-signing` Environment on this repository. The Environment must have a required reviewer who is not the
  person who started the run (prevent self-review), a custom deployment policy that allows tags `v*` only, and hold only this repository's release signing keys: the free-tier plugin key and the agent key (see Agent artifacts below; each is read by its own workflow's signing job (`sign` for plugins, `sign-publish` for the agent), and each run needs its own reviewer approval). The licensed key is never here. A tag ruleset that limits
  creating, updating and deleting `v*` tags to maintainers belongs with it, because the tag decides which code runs next to the key. The owner creates the
  Environment and both secrets (`NSELF_RELEASE_SIGNING_KEY_FREE`, `NSELF_RELEASE_SIGNING_KEY_FREE_ID`) in P7-PROD-59 and checks those settings; this page describes
  what must be true, not what is configured today. No other job can read the secrets.
- The licensed key is never a GitHub secret and never available to this workflow; it stays on the owner's release machine (P7-PLUG-22).
- Rotation: generate the new key offline, commit its public half as `keys/plugins-release-free-<id>.pub` (the `sign` job refuses a key that is not committed), then
  store it as the Environment secrets, and ship a CLI that pins the new key id before the first release signed with it (P7-PLUG-71). Keep the old public key until every supported CLI
  knows the new one. A leaked key goes on the CLI's revoked list instead.
- The private key never appears in a log, an artifact, a file in the repo or an argv.
- A release runs from the tag push. A manual run (`workflow_dispatch`) must start from the tag itself ("Use workflow from: Tag v1.2.3") and enter that same tag;
  the first step refuses anything else, so every job runs the tag's code and the concurrency group is the tag. The Release and the R2 mirror exist before the
  reviewer approves `sign`; trust travels in `releases.json`, which only changes after the signatures exist.
- The `sign` job refuses to run unless `keys/` holds a release public key and the signatures verify against one of them, so a mis-set secret cannot produce a
  signed-looking PR. Commit the public key (P7-PLUG-69) before the first real tag run.
- A manual run from an older tag runs that tag's own workflow file, so this chain cannot backfill releases made before it.

### After a real release

1. Approve the `sign` job in the Actions UI (the required reviewer).
2. Check out the `release-data/<tag>` branch on a maintainer machine that has the private bundles checkout and run `bash scripts/plugin-counts.sh`: it
   regenerates `registry.json`, `catalog.json` and `counts.json` from the new `releases.json`, and the workflow token cannot read the bundles repo.
   Push the result to the branch. That push also starts the required checks, which a pull request opened with `GITHUB_TOKEN` does not. Then merge.
   A re-run never overwrites an existing `release-data/<tag>` branch; delete it first to regenerate. `verify-published-checksums.sh --signatures` refuses to
   write a row over a newer version, so a late run for an older tag cannot roll `releases.json` back.
3. The web `sources-lock` PR is a separate step (P7-PLUG-69).
4. The repository setting "Allow GitHub Actions to create and approve pull requests" (Settings, Actions, General) must stay on; the `release-data` job opens its PR with `GITHUB_TOKEN`.

## Agent artifacts (`nself-ci-agent`)

The CI node agent ships as signed, reproducible binaries on the same GitHub Release as the plugin tarballs (hq NODE D11). `nself ci nodes add`
downloads the pinned version, verifies it and only then installs it; `--agent-binary <path>` stays the way to install a binary you built yourself.

### Files on the Release

| File | Content |
|---|---|
| `nself-ci-agent-<os>-<arch>` | static binary for `linux-amd64`, `linux-arm64` or `darwin-arm64` |
| `nself-ci-agent-<os>-<arch>.manifest.json` | one `agent-release/v1` manifest per binary |
| `nself-ci-agent-<os>-<arch>.cdx.json` | CycloneDX 1.6 SBOM of the binary (pinned syft) |
| `nself-ci-agent-<os>-<arch>.spdx.json` | SPDX 2.3 SBOM of the binary |
| `nself-ci-agent-<os>-<arch>.intoto.dsse.json` | DSSE envelope over the signed in-toto provenance statement |

**Version floor.** Every agent version from P7-CACHE-10 on carries the two SBOMs and the provenance, and the client refuses a release without them (`E665`,
reason `provenance`). Agent versions released between P7-NODE-15 and P7-CACHE-10 carry none and are refused by a newer coordinator. V15 is unreleased until
P7-SHIP-09, so no user depends on those versions.

A manifest is one line of JSON: `schema`, `version`, `target`, `sha256`, `protocol_min`, `protocol_max`, `key_id` (`agent-<16 hex>`), `alg` (`ed25519`) and `sig`
(standard padded base64 of the raw 64-byte signature, the text `sdk/go/signing` `EncodeSig` produces). The signed message is plain text, not JSON:

```
nself-agent-v1\n<target>\n<version>\n<sha256 hex of the binary>\n<protocol_min>-<protocol_max>\n
```

Every field is followed by a newline, the last one too. `manifest.sh` signs it with the same `openssl pkeyutl -sign -rawin` primitive as `sign.sh`. The Go side builds
the same bytes in `free/ci/internal/agent/verify` (`SigningInput`); a test signs with the script and verifies in Go, so the two cannot drift.

### Scripts (`scripts/release/agent/`)

| Script | Use |
|---|---|
| `build.sh --version V --out DIR` | reproducible build for the three targets (`CGO_ENABLED=0`, `-trimpath`, `-buildvcs=false`, empty build id, vendored modules). Writes `SHA256SUMS` and `VERSION`. Two builds of one tree, version and Go toolchain are byte-identical (the log prints `go version`; the workflow pins the toolchain) |
| `manifest.sh --in DIR [--rehearsal]` | writes and signs one manifest per binary. Key from `NSELF_AGENT_SIGNING_KEY` and `NSELF_AGENT_SIGNING_KEY_ID` (environment only; the id is derived from the key and checked). `--rehearsal` uses a throwaway key and writes `DIR/rehearsal-test.pub` |
| `provenance.sh --in DIR [--rehearsal] [--versions FILE]` | after `manifest.sh`: per binary, runs the pinned syft (version and checksums from the `syft` row of `free/ci/internal/toolchain/versions.yaml`; a candidate that is not exactly that binary never runs, and with none the pinned archive is downloaded over https and checked), writes `<binary>.cdx.json` and `.spdx.json`, runs `go run ./tools/provgen` (the statement: buildType `https://nself.org/ci/buildtypes/agent/v1`, subject = binary name and the signed manifest's sha256, byproducts = the SBOM digests, builder = the workflow ref, invocation = the run id), signs the DSSE pre-authentication encoding with the same agent key and the same `openssl pkeyutl -sign -rawin` primitive, and writes `<binary>.intoto.dsse.json`. Key and key id from the same environment variables as `manifest.sh`; builder, run and commit from `GITHUB_WORKFLOW_REF`, `GITHUB_RUN_ID`, `GITHUB_SHA`. `--rehearsal` signs with a second throwaway key and appends its public half to `DIR/rehearsal-test.pub` |
| `genkeys.sh OUTPUT.go` | generates `free/ci/internal/agent/verify/keys_gen.go` from `scripts/release/keys/agent-<16 hex>.pub` (run it with `go generate ./internal/agent/verify/...` in `free/ci`). A drift test fails when the committed file is stale |

`nself-ci-agent verify-release --pub FILE [--version V] [--revoked FILE] [--require-all-targets] DIR` is the operator-side check: it verifies every binary in DIR against its manifest, its SBOMs and its provenance with
the key file (PEM SPKI; one key, or in a rehearsal the manifest key and the provenance key). A binary without a manifest, a manifest without a binary, a missing SBOM or provenance, a symlinked file or an empty directory fails; `--require-all-targets` (set by the workflow) also fails unless all three targets are present, and `--revoked FILE` refuses a manifest signed by a revoked key id.

### Rehearsal (no key, no release, no network)

```bash
bash scripts/release/agent/build.sh --version v0.0.0-rehearsal --out "$TMPDIR/agent-a"
bash scripts/release/agent/manifest.sh --in "$TMPDIR/agent-a" --rehearsal
bash scripts/release/agent/provenance.sh --in "$TMPDIR/agent-a" --rehearsal
(cd free/ci && CGO_ENABLED=0 go run -mod=vendor ./cmd/agent verify-release --pub "$TMPDIR/agent-a/rehearsal-test.pub" "$TMPDIR/agent-a")
```

A one-byte change in a binary fails with `checksum_mismatch`; a changed SBOM or a provenance for other bytes fails with `provenance`. `release-agent.yml` run by hand with `rehearsal` true does the same in Actions and uploads nothing.

### Workflow and key custody

`release-agent.yml` runs on a `v*` tag: `build` (twice, compared; its job output carries the digest of `SHA256SUMS`) and `sign-publish` (bound to the `release-signing` Environment, the only job that reads the agent key). The
workflow uses no Actions artifact and no Actions cache (we never pay GitHub for storage), so nothing travels between jobs but that digest: `sign-publish` rebuilds from the same tag, refuses to go on unless the rebuild has the same digest, and then runs
`manifest.sh`, `provenance.sh` and `verify-release`. The agent key reaches the two signing steps only, and the release token reaches the upload step only; the upload adds the missing assets, never `--clobber`. Both workflows bind the same `release-signing` Environment (Ticket constraint P7-NODE-15), so an approver of one run can in principle trigger the other; a separate Environment for the agent key is a one-line change in `release-agent.yml` if the owner prefers the smaller blast radius. `sign-publish` refuses to continue unless `scripts/release/keys/<key id>.pub` is committed and `verify-release` accepts every manifest
against it. The agent key is a different key from the plugin key (purpose `agent`): a plugin-purpose signature never verifies an agent, and `genkeys.sh` (run by the `sign-publish` job too) refuses an `agent-*.pub` that is the same key as any other committed `.pub`. `sign-publish` also refuses a rebuild that holds files `SHA256SUMS` does not list or a `VERSION` other than the tag's, and the upload step never replaces an asset: a target is released as a set (binary, manifest, SBOMs, then the provenance as the commit marker). A re-run finds a target whose provenance is already on the release and re-verifies the downloaded set with `verify-release` and compares the binary byte for byte (the SBOMs and the provenance carry a timestamp, a serial number and the run id, so they are not reproducible); a target with assets but no provenance stops the run, and the half-finished assets are deleted by hand. A revoked agent key goes into `scripts/release/keys/agent-revoked.txt` (one `agent-<16 hex>` per line); `genkeys.sh` compiles it into the client and `sign-publish` passes it to `verify-release`. `protocol_min` and `protocol_max` are signed and informational for now: no client decision reads them yet. The key and the Environment
secrets are created by the owner; the production public key is committed and the first real release is run by P7-NODE-29. Until then `keys_gen.go` holds an empty key
set and `nodes add` without `--agent-binary` fails with `E665` (reason `no_trusted_key`) naming `--agent-binary`.

### What the client checks (code `E665`)

Before any file is placed, `verify.Artifact` checks in this order: a trusted key exists; the manifest is canonical; the signature is canonical base64; the key id is known,
not revoked, of the agent purpose and valid; the signature verifies over the signing input; the signed version is the pinned version; the signed target is the node's;
the file's sha256 is the signed sha256; then the supply-chain files (`VerifyProvenance`): the three files exist, the DSSE envelope is canonical and verifies under an agent-purpose key (a `ci-release`, `plugins` or unknown key is refused), its payload type is the in-toto one, the statement parses and has the agent build type and the release-agent workflow as builder, its single subject is `nself-ci-agent-<target>` with the signed manifest's sha256, and its two byproducts are the SBOMs with the sha256 of the SBOM bytes that were downloaded. Each failure carries its own reason key: `no_trusted_key`, `malformed_manifest`, `missing_signature`, `malformed_signature`,
`unknown_key`, `wrong_purpose`, `revoked`, `key_not_valid`, `bad_signature`, `version_mismatch`, `target_mismatch`, `checksum_mismatch`, `unreadable_artifact`, `provenance`. `nodes add` and the upgrade path both reach this one call (`verify.Artifact`); no second verify package exists. Downloads
use https (plain http only for a loopback host), are size-bounded and include the three supply-chain files (a 404 on one of them is `provenance`, any other download failure is `E656`); the verified binary is moved into place with one rename, so an unverified file never sits at the
final path, in a fresh private directory per install (no persistent cache). An agent already installed on the node is used as it is when `--agent-binary` is not given (no download and no verification of the existing file); `nodes add` only installs when the node has none. `NSELF_CI_AGENT_RELEASE_URL` points the download at a mirror or a local fixture directory.
