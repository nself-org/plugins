# Changelog

All notable changes to plugin-sdk-go are documented here.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [0.1.0] - 2026-04-23

### Added

Initial public release. Extracted from nSelf private plugin infrastructure.

- `plugin` — `Plugin` interface, `Info` struct, `Base` embed, `Validate()`
- `logger` — JSON `slog.Logger` factory with `plugin` + `version` attrs
- `config` — env-driven config loader with validation helpers
- `server` — chi router with `/healthz`, `/readyz`, `/metrics`, `/version`
- `metrics` — shared Prometheus registry + per-plugin request/error counters
- `license` — offline license cache, grace period, skip-verify dev flag
- `httpx` — HTTP client with retries, timeouts, propagated request-ID header
- `db` — `pgxpool` connect/health/migration helpers
- `tracing` — OpenTelemetry tracer + request-ID middleware
- `middleware` — request-ID injection, common validation helpers
- `costmeter` — shared cost accounting for AI/inference plugins
- `identity` — Ed25519 per-plugin keypair + request signing/verification
- `testing` — test harness: `StubUpstream`, `DoJSONRequest`, `FetchMetrics`, `AssertHealthEndpoints`
- `devkit/cmd/new-plugin` — scaffolding generator for new plugin projects
- `compatibility.go` — `CheckMinSDK` and `CheckCLICompat` runtime guards
- `doc.go` — `Version` constant (`"0.1.0"`)
- CI workflow: test (Go 1.23/1.24), govulncheck, private-import-path security check

### Notes

- Requires Go 1.23 or later.
- Targets nSelf CLI v1.0.9 and newer.
- Zero dependencies on `plugins-pro` private code.

[0.1.0]: https://github.com/nself-org/cli/sdk/go/releases/tag/v0.1.0


## [0.1.1] - 2026-04-25

### Fixed
- **`CheckCLICompat`**: updated minimum compatible CLI version constant to `v1.0.12`.
- **`registry` package**: plugin registry endpoint defaulted to `plugins.nself.org` — now validates TLS cert fingerprint on first connection.

### Changed
- SPORT F01 plugin count updated to 25 free + 87 paid (87 confirmed via plugins-pro schema validator run in P96).
