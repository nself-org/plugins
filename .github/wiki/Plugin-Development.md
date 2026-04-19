# Plugin Development

This page walks through building a new nSelf plugin from empty directory to shipped service. It targets free + pro plugins written in Go against [`plugin-sdk-go`](https://github.com/nself-org/plugin-sdk-go).

See also:
- [[Home]]
- [[Contributing]]
- Compatibility matrix: `.claude/docs/plugins/compatibility.md` (ecosystem doc)
- Scope doctrine: `.claude/docs/doctrines/plugin-scope.md`

---

## 1. Scaffold a Plugin

The CLI generates the full directory layout for you:

```bash
nself plugin new my-notifier
```

Flags:

```bash
nself plugin new my-notifier --template go          # default
nself plugin new my-notifier --template node
nself plugin new my-notifier --template rust        # CPU hot-path only
nself plugin new my-notifier --description "Push notification fan-out"
nself plugin new my-notifier --author "You <you@example.com>"
nself plugin new my-notifier --license MIT
```

The Go template produces:

```text
my-notifier/
  plugin.json
  README.md
  Dockerfile
  docker-compose.plugin.yml
  migrations/
    0001_init.sql
  go/
    go.mod
    cmd/main.go
    internal/
      config/config.go
      server/server.go
```

The `go.mod` pre-pins `github.com/nself-org/plugin-sdk-go` so the SDK is ready to import.

---

## 2. Read the Scope Rules

Before writing code, read `.claude/docs/doctrines/plugin-scope.md`. One domain per plugin, one schema (`np_my_notifier`), one Docker image, mandatory `/healthz` + `/readyz` + `/metrics` + `/version` endpoints. Violations are rejected at code review.

---

## 3. Use the SDK

```go
package main

import (
    "context"
    "log/slog"
    "net/http"
    "os"

    sdk "github.com/nself-org/plugin-sdk-go"
    "github.com/nself-org/plugin-sdk-go/config"
    "github.com/nself-org/plugin-sdk-go/db"
    "github.com/nself-org/plugin-sdk-go/logger"
    "github.com/nself-org/plugin-sdk-go/metrics"
    "github.com/nself-org/plugin-sdk-go/server"

    "github.com/go-chi/chi/v5"
)

const (
    pluginName = "my-notifier"
    version    = "0.1.0"
    minSDK     = "0.1.0"
)

type state struct {
    log *slog.Logger
    met *metrics.Registry
}

func (s *state) Ready(ctx context.Context) error { return nil }

func main() {
    if err := sdk.CheckMinSDK(minSDK); err != nil {
        slog.Error("sdk check failed", "err", err)
        os.Exit(1)
    }

    log := logger.New(logger.Options{
        Plugin:  pluginName,
        Version: version,
        Level:   logger.ParseLevel(config.Env("LOG_LEVEL", "info")),
    })

    ctx := context.Background()
    dsn, err := config.EnvRequired("DATABASE_URL")
    if err != nil {
        log.Error("config", "err", err)
        os.Exit(1)
    }
    pool, err := db.Open(ctx, db.PoolConfig{DSN: dsn})
    if err != nil {
        log.Error("db open", "err", err)
        os.Exit(1)
    }
    defer pool.Close()

    met := metrics.NewRegistry(pluginName, version)
    st := &state{log: log, met: met}

    r := server.New(server.Options{
        Plugin:  pluginName,
        Version: version,
        Metrics: met,
        Ready:   st,
        Routes: func(r chi.Router, m *metrics.Registry) {
            r.With(m.Middleware("/v1/send")).Post("/v1/send", st.handleSend)
        },
    })

    addr := ":" + config.Env("PORT", "8080")
    log.Info("listening", "addr", addr)
    if err := http.ListenAndServe(addr, r); err != nil {
        log.Error("serve", "err", err)
        os.Exit(1)
    }
}

func (s *state) handleSend(w http.ResponseWriter, r *http.Request) {
    // your logic
    w.WriteHeader(http.StatusAccepted)
}
```

Four free endpoints appear without you writing them:

```bash
curl http://localhost:8080/healthz
curl http://localhost:8080/readyz
curl http://localhost:8080/metrics
curl http://localhost:8080/version
```

---

## 4. Metrics: Universal + Your Own

`metrics.NewRegistry()` ships:

- `nself_plugin_requests_total{plugin, route, method, status}`
- `nself_plugin_request_duration_seconds{plugin, route, method}`
- `nself_plugin_in_flight_requests{plugin}`
- `nself_plugin_errors_total{plugin, kind}`
- `nself_plugin_build_info{plugin, version}`

Register your own metrics against the same registry:

```go
sendLatency := prometheus.NewHistogram(prometheus.HistogramOpts{
    Name: "nself_my_notifier_send_latency_seconds",
    Help: "Time to deliver a notification upstream.",
})
met.Reg.MustRegister(sendLatency)
```

The nSelf monitoring bundle (Prometheus + Grafana + Loki) scrapes every plugin's `/metrics` automatically — no extra config.

---

## 5. Logging: slog Only

Use the SDK logger factory. Never `log.Printf`. Output is JSON with `plugin` and `version` keys so Loki / Grafana can filter by plugin cleanly.

```go
log := logger.New(logger.Options{Plugin: "my-notifier", Version: "0.1.0"})
log.Info("delivered", "channel", "fcm", "tokens", 123, "duration_ms", 45)
log.Error("upstream 5xx", "err", err, "retry", 2)
```

`LOG_LEVEL=debug` enables verbose output. All plugins must honor this env var.

---

## 6. License (Pro Plugins Only)

Free plugins: skip this section.

Pro plugins rely on the CLI loader to gate install. At runtime, use `sdk/license` only if you need to re-check a long-running entitlement (e.g. a daily cron):

```go
import "github.com/nself-org/plugin-sdk-go/license"

v := license.NewValidator(os.Getenv("HOME") + "/.nself/license/cache.json")
if err := v.AllowPlugin("my-notifier", os.Getenv("NSELF_PLUGIN_LICENSE_KEY"), time.Now()); err != nil {
    log.Error("license", "err", err)
    os.Exit(1)
}
```

The validator honors `NSELF_LICENSE_SKIP_VERIFY=1` in dev, and tolerates ping.nself.org outages up to the grace period (default 7 days).

---

## 7. Testing

Use the SDK's test helpers:

```go
package server

import (
    stdtesting "testing"
    sdktest "github.com/nself-org/plugin-sdk-go/testing"
)

func TestSend(t *stdtesting.T) {
    upstream := sdktest.StubUpstream(t, map[string]any{
        "/fcm/v1/send": map[string]string{"name": "ok"},
    })
    defer upstream.Close()

    h := newHandler(upstream.URL)
    status, body := sdktest.DoJSONRequest(t, h, "POST", "/v1/send", map[string]any{
        "token": "abc",
        "body":  "hello",
    })
    if status != 202 {
        t.Errorf("status=%d body=%v", status, body)
    }
}
```

Target ≥70% line coverage on `internal/`. CI fails below.

---

## 8. Manifest: `plugin.json`

Minimum fields:

```json
{
  "name": "my-notifier",
  "version": "0.1.0",
  "description": "Push notification fan-out.",
  "category": "communication",
  "tier": "free",
  "language": "go",
  "min_cli": "1.0.6",
  "max_cli": "",
  "min_sdk": "0.1.0",
  "dependencies": [],
  "systemDependencies": {
    "postgres": ">=16",
    "redis": ">=7"
  },
  "configSchema": {
    "DATABASE_URL": {"type": "string", "required": true},
    "REDIS_URL":    {"type": "string", "required": true},
    "LOG_LEVEL":    {"type": "string", "default": "info"}
  }
}
```

Pro plugins additionally set:

```json
{
  "tier": "pro",
  "isCommercial": true,
  "licenseType": "pro",
  "requires_license": true,
  "requiredEntitlements": ["pro"]
}
```

---

## 9. Hot Reload (Dev)

Running the plugin via `nself plugin dev my-notifier` watches your source tree and rebuilds on change. Internally, the loader sets `NSELF_DEV_HOT_RELOAD=1`; the SDK's server mounts a `/debug/reload` endpoint that triggers a graceful restart. See the [[Hot-Reload-Dev]] page for details.

---

## 10. Submit + Publish

```bash
# Verify the plugin locally
nself plugin verify my-notifier

# Package it
nself plugin package my-notifier

# Submit (free plugins → plugins repo PR; pro plugins → plugins-pro via invite)
nself plugin submit my-notifier
```

The PR template requires:

1. Manifest passes `nself plugin verify`.
2. README covers install, config, troubleshooting.
3. Tests + CI green.
4. Compatibility matrix entry added in the PPI doc.

---

## Bottom Navigation

- Back to [[Home]]
- [[Contributing]]
- [[Plugin-Marketplace]]
