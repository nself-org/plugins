package rest

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers"
)

func config(reason string) *providers.Error {
	return providers.NewError(providers.Config, "E702", reason, 0, time.Time{})
}

func classifyNetwork(err error) *providers.Error {
	var uerr *url.Error
	if errors.As(err, &uerr) {
		err = uerr.Err
	}
	var pe *providers.Error
	if errors.As(err, &pe) {
		return pe
	}
	if errors.Is(err, errPrivate) {
		return config("ssrf.private")
	}
	return providers.NewError(providers.Transient, "E704", "network", 0, time.Time{})
}

func classify(status int, header http.Header, body []byte, now time.Time) *providers.Error {
	class, code, reason := providers.Config, "E702", "request"
	var retry time.Time
	switch {
	case status == 401:
		class, code, reason = providers.Auth, "E701", "credential"
	case status == 403 && (header.Get("X-RateLimit-Remaining") == "0" || strings.Contains(strings.ToLower(string(body)), "secondary rate limit")):
		class, code, reason = providers.Quota, "E703", "rate_limit"
	case status == 403:
		class, code, reason = providers.Auth, "E701", "permission"
		if scope := header.Get("X-Accepted-OAuth-Scopes"); scope != "" {
			reason = "missing_scope:" + strings.TrimSpace(strings.Split(scope, ",")[0])
		} else if scope := header.Get("X-OAuth-Scopes"); scope != "" {
			reason = "scope:" + strings.TrimSpace(strings.Split(scope, ",")[0])
		}
	case status == 429:
		class, code, reason = providers.Quota, "E703", "rate_limit"
	case status >= 500:
		class, code, reason = providers.Transient, "E704", "server"
	}
	if class == providers.Quota {
		if seconds, err := strconv.Atoi(header.Get("Retry-After")); err == nil && seconds >= 0 {
			retry = now.Add(time.Duration(seconds) * time.Second)
		}
		if retry.IsZero() {
			if date, err := http.ParseTime(header.Get("Retry-After")); err == nil {
				retry = date
			}
		}
		if retry.IsZero() {
			if epoch, err := strconv.ParseInt(header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
				retry = time.Unix(epoch, 0)
			}
		}
	}
	return providers.NewError(class, code, reason, status, retry)
}
