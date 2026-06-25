package internal

// ssrf.go — SSRF guard delegation for notify webhook delivery.
//
// SECURITY (Security-Always-Free Doctrine): user-supplied webhook URLs are
// untrusted. This file provides the package-local validateWebhookURL function
// which delegates to the canonical shared guard in
// github.com/nself-org/plugin-sdk/httpx.ValidateOutboundURL.
//
// The canonical implementation lives in the shared SDK so every plugin that
// delivers outbound HTTP requests uses exactly one guard implementation.
//
// Inputs:    raw webhook URL string.
// Outputs:   nil when safe to deliver, descriptive error otherwise.
// Constraints: dev opt-in via NSELF_ALLOW_PRIVATE_URLS=true (never in prod).

import (
	"net/http"
	"time"

	sdkhttpx "github.com/nself-org/plugin-sdk/httpx"
)

// validateWebhookURL delegates to the canonical shared SSRF guard. See
// github.com/nself-org/plugin-sdk/httpx.ValidateOutboundURL for full details.
// Plugin-local env opt-in NOTIFY_ALLOW_PRIVATE_URLS is superseded by the
// canonical NSELF_ALLOW_PRIVATE_URLS flag in the shared guard.
func validateWebhookURL(rawURL string) error {
	return sdkhttpx.ValidateOutboundURL(rawURL)
}

// ssrfSafeClient is an http.Client that refuses to follow redirects, so a
// public destination cannot bounce a request to a validated-around internal
// address.
var ssrfSafeClient = &http.Client{
	Timeout: 30 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	},
}
