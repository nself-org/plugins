// Package httptimeout provides a timeout-bounded default *http.Client,
// mirroring the one function of cli/internal/httptimeout this plugin uses.
//
// Constraints: cli/internal/httptimeout is unreachable from this plugin
// module and is used broadly across the core CLI (27+ call sites, several
// scopes with independently-tunable env vars) — copying the whole package
// for the one scope (Default) this plugin needs would be a much larger
// surface than warranted. Only NSELF_HTTP_TIMEOUT_DEFAULT is honored here.
package httptimeout

import (
	"net/http"
	"os"
	"strconv"
	"time"
)

// Default is the general-purpose client used by claw_pair.go and
// claw_mcp.go, matching core's 30s default scope.
var Default = &http.Client{Timeout: envDuration("NSELF_HTTP_TIMEOUT_DEFAULT", 30*time.Second)}

func envDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return time.Duration(n) * time.Second
}
