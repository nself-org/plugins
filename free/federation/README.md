# federation

Manage GraphQL Federation for nSelf: compose an Apollo Router supergraph
from installed plugin subgraphs, check subgraph health, and introspect the
composed schema.

**Tier:** Free (MIT) — no license required. Federation is opt-in
(`NSELF_FEDERATION=true` in your `.env`), not a licensed feature.

## Installation

```bash
nself plugin install federation
```

This is a CLI-proxy plugin, not a long-running service: there is no port, no
HTTP server, and no database table of its own. Installing it places the
`nself-federation` binary at `~/.nself/plugins/bin/nself-federation`. From
then on, `nself federation ...` routes to it exactly as it did when
`federation` was a core command (pre-CLI-R11).

## Usage

```bash
nself federation compose        # re-compose the supergraph from installed plugin subgraphs
nself federation status         # subgraph health and composition state
nself federation status --json
nself federation introspect     # print the composed supergraph.graphql
```

`compose` and `status` resolve the current nSelf project (found by walking
up from the working directory), read `NSELF_FEDERATION`, `NSELF_PLUGIN_DIR`,
and `HASURA_PORT` from the same `.env` cascade core uses, scan
`NSELF_PLUGIN_DIR` for installed plugins with a `graphql.enabled: true`
block, and shell out to `rover supergraph compose`.

## History

Extracted from `cli/cmd/commands/federation.go` under CLI-R11. The domain
package (`internal/federation`: compose/registry/router/types, tests
included) moved wholesale, unchanged. Three small pieces the CLI's
`internal/*` packages provided were reimplemented standalone, since the
plugin cannot import them across the module boundary:

- `internal/tui` in place of the CLI's `internal/ui` (terminal output).
- `internal/projectroot` in place of `internal/config.FindNSelfRoot`.
- `internal/envcascade` in place of the three config fields
  (`FederationEnabled`, `PluginSystem.Dir`, `Hasura.Port`) that
  `config.Load` would otherwise populate — reusing `github.com/joho/godotenv`
  (the same third-party library core depends on) to apply the identical
  CLI-R18 `.env` cascade order, rather than guessing at env resolution.
- `internal/manifest` in place of `internal/plugin.LoadManifestsFromDir` —
  a minimal plugin.json reader carrying only the five fields
  (`name`, `port`, `graphql.*`) federation actually needs, instead of the
  full ~40-field manifest schema. See the package doc comment for the one
  known, narrow behavior difference this introduces.

## Development

```bash
go build -o nself-federation ./cmd/
go test ./...
```
