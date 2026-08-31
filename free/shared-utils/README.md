# shared-utils

Internal Go utilities shared by multiple free nself plugins. MIT licensed.
Not installable directly (`"installable": false` in `plugin.json`) — it is a
library dependency, not a plugin a user runs.

## Why this exists

`plugins-pro/paid/shared` is a Source-Available, internally-licensed module
used by paid plugins. Several free (MIT) plugins previously imported it
directly for small utility packages, which was both a build break (the
`replace ../shared` target did not exist in this repo) and a licensing
violation: an MIT plugin cannot depend on Source-Available code.

`free/shared-utils` is the free counterpart: the same utility surface,
reimplemented as this repo's own MIT-licensed code, so free plugins never
link paid code. `plugins-pro/paid/shared` keeps its own copies and its own
license for the paid plugins that still use it — the two are deliberately
not merged.

Named `shared-utils` rather than `shared` because this repo's
`shared/validate-registry.sh` treats any directory literally named `shared`
as non-plugin scaffolding (to skip) — a real, registry-tracked plugin
directory needs a different name.

See P6 ADR-P6-02 and `.claude/inbox/msg-2026-08-31-plugins-free-depend-on-paid-shared.md`.

## Packages

- `httpmid` — request-ID tracing middleware (`RequestIDMiddleware`).
- `httpclient` — HTTP clients that propagate the request ID set by `httpmid`.
- `go/server` — `GracefulShutdown` helper for HTTP server lifecycle.

## Usage

```go
import "github.com/nself-org/plugins/free/shared-utils/httpclient"

client := httpclient.New(httpclient.Options{})
```

Consuming plugins reference this module via a local `replace` directive:

```
require github.com/nself-org/plugins/free/shared-utils v0.0.0
replace github.com/nself-org/plugins/free/shared-utils => ../shared-utils
```
