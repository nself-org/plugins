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
  person who started the run (prevent self-review), a custom deployment policy that allows tags `v*` only, and hold no other signing key. A tag ruleset that limits
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
