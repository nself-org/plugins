# Plugin Images

Free compose plugins are published as multi-arch container images. A generated lock file, `images.json`,
records every image the plugins repo ships or depends on, pinned by digest. The CLI image lock ingests it.

## Naming and platforms

- Image: `nself/nself-<slug>:<version>`, plus `:latest`.
- Platforms: `linux/amd64` and `linux/arm64`, one OCI index per tag, no attestation entries.
- Labels: `org.opencontainers.image.source`, `.revision`, `.version`.
- Eligible plugins (`scripts/images/matrix.sh`): free, `manifest_version` 2, `service.kind` compose, with a Dockerfile.
  Unconverted plugins are skipped by rule.
- Licensed plugins publish no public image. They build locally from the licence-gated tarball.

## images.json v1

```json
{"schema": "nself.plugins.images/v1", "_generated": "...", "images": {"<name>": {"kind", "image", "platforms", "owner"}}}
```

It is assembled from per-owner fragments in `images.d/` by `scripts/images/assemble.sh` (sorted keys, a duplicate
name, a reference without `@sha256:` or an unknown kind fails and names the fragment). `assemble.sh --check`
fails when a fragment changed without re-assembly. See `images.d/README.md` for how an owner adds an entry.
The file is empty (`"images": {}`) until the first publish.

## Pinning

Consumers read images from the lock, never from a floating tag. The weekly bump re-runs the probe.

## Daily probe

`images-probe.yml` runs daily and on pull requests that touch `images.d/`, `images.json` or `scripts/images/`.
`scripts/images/probe.sh` pulls each entry's index anonymously (empty Docker config, no credential), requires the bytes
to hash to the pinned digest and to list every platform. Calls are time-limited and retried. A definite failure
(image gone, unpullable, wrong digest, missing platform) fails the job and, on the schedule, opens or updates one
`image-probe` issue. A broken probe tool or an unrecognised registry error also fails; a definite registry answer always wins over transient words in the same message. Only a rate limit, outage or timeout is inconclusive: it warns, never blocks a pull request, and the third inconclusive scheduled run in a row opens the issue too.

## Publishing

`plugin-images.yml` runs on a release tag (vX.Y.Z; pre-release tags and versions never move `:latest`). It builds the matrix and pushes only when the repository variable
`NSELF_PUBLISH_PLUGIN_IMAGES` is `true`; otherwise it builds, pushes nothing and logs
`skipped push: NSELF_PUBLISH_PLUGIN_IMAGES != true`. A push also needs `DOCKERHUB_TOKEN` and `DOCKERHUB_USERNAME`
(variable). The workflow then writes only `images.d/plugins.json` and the re-assembled `images.json`, and opens a PR.
The first real publish is a separate, owner-named Ticket (P7-PLUG-65).

## Local tools

| Script | Purpose |
|---|---|
| `matrix.sh [--json]` | eligible plugins |
| `build.sh --plugins a,b --push=false --out DIR` | multi-arch build, OCI export, index check (needs a docker-container buildx builder with QEMU) |
| `assemble.sh [--check]` | write or verify `images.json` |
| `probe.sh` | anonymous availability probe |
| `record.sh --meta DIR` | the workflow's write step |

Each tool has `--fixtures` that proves it against planted defects.
