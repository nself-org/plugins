package forgejo

// Purpose: the v1.5 Forgejo check behind `nself ci forgejo` (P7-HOST-12, Epic D14): the server health
// endpoint and the optional admin-authenticated API probe, with the base URL taken from the operator
// policy key providers.forgejo.url. Unlike the v1.4 command (forgejo.go, frozen) it has no runner
// container check: P7-TRUST-21 removed that runner.
// Inputs:  CheckConfig (URL, optional admin user and password, HTTP client); the policy key.
// Outputs: Result, and the status lines on a writer; Deprecation, the one line the alias prints.
// Constraints: the admin password is read from the environment by the caller and goes only into the
//          basic-auth header of one GET; it is never printed, logged or put in Result. Plain text
//          only (no colour, no framing), so the output is the same on a terminal and in a pipe. The
//          policy key is operator scope (Spend), so a repository cannot choose the host this probes.
//          A server that answers is healthy only when /-/health says so; every failure is a result,
//          never a panic.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/policy"
)

// KeyURL is the policy key that holds the Forgejo base URL.
const KeyURL = "providers.forgejo.url"

// Deprecation is the one line the V15 alias prints before the check.
const Deprecation = "nself ci forgejo is deprecated and is removed in v1.6: run `nself ci doctor --providers` (set the policy key " + KeyURL + " to include Forgejo)"

// checkTimeout bounds each request of the check.
const checkTimeout = 10 * time.Second

// Keys returns the spec of the one key this file owns. Kind is TightenFixed with no registered
// default (the default lives in DefaultURL), as for the other providers.* keys.
func Keys() []policy.KeySpec {
	return []policy.KeySpec{{Key: KeyURL, Owner: "P7-HOST-12", Kind: policy.TightenFixed, Spend: true, Decode: decodeURL}}
}

var registrationErrors []error

// RegistrationErrors returns what init() could not register; empty when the key is registered.
func RegistrationErrors() []error { return append([]error(nil), registrationErrors...) }

func init() {
	for _, spec := range Keys() {
		if err := policy.RegisterKey(spec); err != nil {
			registrationErrors = append(registrationErrors, err)
		}
	}
}

func decodeURL(raw json.RawMessage) (any, error) {
	var v any
	d := json.NewDecoder(bytes.NewReader(raw))
	if err := d.Decode(&v); err != nil || d.More() {
		return nil, fmt.Errorf("E672 %s: expected exactly one JSON value", KeyURL)
	}
	s, ok := v.(string)
	if !ok {
		return nil, fmt.Errorf("E672 %s: expected a URL string", KeyURL)
	}
	if err := ValidateURL(s); err != nil {
		return nil, fmt.Errorf("E672 %s: %w", KeyURL, err)
	}
	return strings.TrimRight(s, "/"), nil
}

// ValidateURL accepts an http or https base URL with a host and no user info, query or fragment.
func ValidateURL(s string) error {
	u, err := url.Parse(s)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("expected an http or https base URL with a host and no user info, query or fragment")
	}
	return nil
}

// URLFrom reads the key from the effective policy; DefaultURL when the operator set none.
func URLFrom(eff *policy.Effective) (string, error) {
	if eff == nil {
		return "", fmt.Errorf("forgejo: no effective policy")
	}
	v, _, ok := eff.Value(KeyURL)
	if !ok {
		return DefaultURL, nil
	}
	s, isStr := v.(string)
	if !isStr || ValidateURL(s) != nil {
		return "", fmt.Errorf("E672 %s: effective value is not a valid URL", KeyURL)
	}
	return strings.TrimRight(s, "/"), nil
}

// CheckConfig is what Check needs. A nil Client gets one with a 10 s timeout.
type CheckConfig struct {
	URL        string
	User, Pass string
	Client     *http.Client
}

// Result is the outcome of one check. API is "not configured", "reachable (admin authenticated)",
// "unreachable" or "HTTP <status>"; it never carries a credential.
type Result struct {
	URL     string `json:"url"`
	Healthy bool   `json:"healthy"`
	Server  string `json:"server"`
	API     string `json:"api"`
}

// APINotConfigured is the API line when no admin user and password were given.
const APINotConfigured = "not configured (set NSELF_FORGEJO_ADMIN_USER and NSELF_FORGEJO_ADMIN_PASSWORD)"

// Check probes the server, then the admin API when both credentials are given and the server is up.
func Check(ctx context.Context, cfg CheckConfig) Result {
	base := strings.TrimRight(cfg.URL, "/")
	if base == "" {
		base = DefaultURL
	}
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: checkTimeout}
	}
	res := Result{URL: base, API: APINotConfigured}
	if ValidateURL(base) != nil {
		res.URL, res.Server = redacted(base), "invalid URL"
		return res
	}
	res.Healthy, res.Server = serverState(ctx, client, base)
	if res.Healthy && cfg.User != "" && cfg.Pass != "" {
		res.API = apiState(ctx, client, base, cfg.User, cfg.Pass)
	}
	return res
}

// redacted is base without user info, so a URL refused for carrying a password never echoes it.
func redacted(base string) string {
	u, err := url.Parse(base)
	if err != nil {
		return "(unparseable)"
	}
	u.User, u.RawQuery, u.Fragment = nil, "", ""
	return u.String()
}

func serverState(ctx context.Context, client *http.Client, base string) (bool, string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/-/health", nil)
	if err != nil {
		return false, "unreachable"
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, "unreachable"
	}
	defer func() { _ = resp.Body.Close() }()
	var h struct {
		Healthy bool `json:"healthy"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&h) == nil && h.Healthy {
		return true, "healthy"
	}
	return false, fmt.Sprintf("HTTP %d", resp.StatusCode)
}

func apiState(ctx context.Context, client *http.Client, base, user, pass string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v1/repos/search?limit=0", nil)
	if err != nil {
		return "unreachable"
	}
	req.SetBasicAuth(user, pass)
	// The password goes to the configured server only: a redirect (for example https to http on the same
	// host) is an answer, not a place to follow it to.
	noFollow := *client
	noFollow.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := noFollow.Do(req)
	if err != nil {
		return "unreachable"
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusOK {
		return "reachable (admin authenticated)"
	}
	return fmt.Sprintf("HTTP %d", resp.StatusCode)
}

// Print writes the status lines of r.
func Print(w io.Writer, r Result) {
	mark := "✗"
	if r.Healthy {
		mark = "✓"
	}
	_, _ = fmt.Fprintf(w, "%s Forgejo server   %s   %s\n", mark, r.URL, r.Server)
	_, _ = fmt.Fprintf(w, "  API auth         %s\n", r.API)
	if !r.Healthy {
		_, _ = fmt.Fprintln(w, "Forgejo server is not healthy. Is the ops profile running? Try: nself start --profile ops")
	}
}
