# images.d: image lock fragments

`images.json` (repository root) is generated. Never edit it by hand. Every image owner writes **one fragment
file here** and runs `bash scripts/images/assemble.sh`; the script merges all fragments into `images.json`.

## Fragment format

One JSON object per file (`images.d/<owner>.json`), or an array of them (the release workflow writes
`images.d/plugins.json` that way):

```json
{
  "name": "minio",
  "kind": "upstream",
  "image": "quay.io/minio/minio:RELEASE.2025-01-01T00-00-00Z@sha256:<64 hex>",
  "platforms": ["linux/amd64", "linux/arm64"],
  "owner": "P7-XXX-NN"
}
```

| Key | Rule |
|---|---|
| `name` | Unique across all fragments. Plugins use their slug. |
| `kind` | `plugin`, `upstream` or `ci`. |
| `image` | `<repository>:<tag>@sha256:<index digest>`. A tag without a digest fails. |
| `platforms` | Platforms the index must list, for example `linux/amd64`. |
| `owner` | Ticket id or workflow that owns the entry. |

The format is a contract: later changes are additive only.

## Adding an entry

1. Write `images.d/<owner>.json`. Do not touch another owner's file.
2. `bash scripts/images/assemble.sh` (after a rebase, run it again).
3. Commit the fragment and `images.json` together. `assemble.sh --check` fails on drift.
4. `bash scripts/images/probe.sh` pulls every entry anonymously and checks digest and platforms.

`plugins.json` is owned by `plugin-images.yml`; do not edit it. Licensed plugins publish no public image.
