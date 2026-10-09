package conformance

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers"
)

// ClassifyFault maps observed provider failures into safe, typed diagnostics.
func ClassifyFault(status int, header http.Header, networkErr error, now time.Time) *providers.Error {
	if networkErr != nil {
		return providers.NewError(providers.Transient, "E704", "network", 0, time.Time{})
	}
	class, code, reason := providers.Config, "E702", "request"
	var retry time.Time
	switch {
	case status == 401:
		class, code, reason = providers.Auth, "E701", "credential"
	case status == 403 && header.Get("X-RateLimit-Remaining") == "0":
		class, code, reason = providers.Quota, "E703", "rate_limit"
	case status == 403:
		class, code, reason = providers.Auth, "E701", "permission"
		if scope := header.Get("X-Accepted-OAuth-Scopes"); scope != "" {
			reason = "missing_scope:" + strings.TrimSpace(strings.Split(scope, ",")[0])
		}
	case status == 429:
		class, code, reason = providers.Quota, "E703", "rate_limit"
	case status >= 500:
		class, code, reason = providers.Transient, "E704", "server"
	}
	if class == providers.Quota {
		if seconds, err := strconv.Atoi(header.Get("Retry-After")); err == nil {
			retry = now.Add(time.Duration(seconds) * time.Second)
		}
	}
	return providers.NewError(class, code, reason, status, retry)
}
