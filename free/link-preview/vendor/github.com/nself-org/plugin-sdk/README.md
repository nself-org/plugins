# plugins/sdk — Vendored from cli/sdk/go

This directory is a vendored copy of [`nself-org/cli/sdk/go`](https://github.com/nself-org/cli/tree/main/sdk/go) for plugin authoring.

The module name here is `github.com/nself-org/plugin-sdk` (NOT `github.com/nself-org/cli/sdk/go`) to keep existing plugin `go.mod` replace directives working within this repo.

## Why this exists

Plugin `go.mod` files use a `replace` directive:

```
replace github.com/nself-org/plugin-sdk => ../../sdk
```

This allows `go build` to resolve the SDK locally within the plugins repo without requiring a published Go module. CI checks out only this repo, so the SDK must live here.

## Syncing from cli/sdk/go

When `cli/sdk/go` is updated, re-vendor here:

```bash
cp -R /path/to/nself/cli/sdk/go/. /path/to/nself/plugins/sdk/
# Then re-set the module name (cp overwrites go.mod with the cli canonical name):
sed -i '' 's|module github.com/nself-org/cli/sdk/go|module github.com/nself-org/plugin-sdk|' sdk/go.mod
```

Or run the helper (once created in P97):

```bash
make sync-sdk
```

## Long-term plan (P97+)

Publish `cli/sdk/go` to pkg.go.dev as `github.com/nself-org/cli/sdk/go`, then update each plugin's `go.mod` to reference that published path directly. Once all plugins are updated, remove this vendored directory and the `replace` directives.

## Origin

Vendored from `cli/sdk/go` as part of P96 Wave 1C critical fix (2026-04-25).
The migration that moved the SDK from `plugins/sdk/` to `cli/sdk/go/` left this directory as a README-only placeholder, breaking all 22 plugin builds.
