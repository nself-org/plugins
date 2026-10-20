package cloudvm

// Purpose: the contract with the `nself infra server` commands (P7-SURF-29 envelopes): build the
// fixed argv of provision, list and destroy, run it through the funnel with the token in the one
// child's environment, and read the v1 JSON envelope that comes back.
// Inputs:  verb arguments built by this package; the token read from the token file.
// Outputs: servers as burst.Resource, or a classified error.
// Constraints: argv is fixed, absolute for the binary, and never holds the token (--token-env names
// the variable, the token flag is never used); the child environment is NSELF_CI_BURST_TOKEN only;
// the envelope is found by decoding JSON documents that start a line, because the funnel merges
// stdout and stderr; an error envelope is classified by its code and cause (E745 not found, E747
// token, E744 with HTTP 429 quota, 5xx or network error unavailable); a detail carries the code and
// the short cause, never the provider's text.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/netip"
	"strconv"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/burst"
)

// TokenEnv is the one variable the infra child reads the token from.
const TokenEnv = "NSELF_CI_BURST_TOKEN"

// envelope is the part of the v1 envelope this package reads.
type envelope struct {
	SchemaVersion string          `json:"schema_version"`
	Data          json.RawMessage `json:"data"`
	Error         *struct {
		Code  string `json:"code"`
		Cause string `json:"cause"`
	} `json:"error"`
}

// wireServer is one server in the infra JSON.
type wireServer struct {
	ID      int64             `json:"id"`
	Name    string            `json:"name"`
	Created time.Time         `json:"created"`
	IPv4    string            `json:"ipv4"`
	IPv6    string            `json:"ipv6"`
	Labels  map[string]string `json:"labels"`
}

// parseEnvelope returns the last v1 envelope in out.
func parseEnvelope(out []byte) (envelope, bool) {
	var best envelope
	found := false
	for i := 0; i < len(out); i++ {
		if out[i] != '{' || i > 0 && out[i-1] != '\n' {
			continue
		}
		var e envelope
		if json.NewDecoder(bytes.NewReader(out[i:])).Decode(&e) == nil && e.SchemaVersion == "1" && (e.Data != nil || e.Error != nil) {
			best, found = e, true
		}
	}
	return best, found
}

func (w wireServer) resource(now time.Time) burst.Resource {
	r := burst.Resource{ID: strconv.FormatInt(w.ID, 10), Name: w.Name, Labels: map[string]string{}, CreatedAt: w.Created}
	for k, v := range w.Labels {
		r.Labels[k] = v
	}
	if r.CreatedAt.IsZero() {
		r.CreatedAt = now
	}
	for _, a := range []string{w.IPv4, w.IPv6} {
		if ip, err := netip.ParseAddr(a); err == nil {
			r.Addrs = append(r.Addrs, ip)
		}
	}
	return r
}

// classify turns an error envelope into the lifecycle's error classes.
func classify(e envelope, verb string) error {
	code, cause := e.Error.Code, e.Error.Cause
	detail := "infra " + verb + " answered " + code
	if safeCause(cause) {
		detail += " (" + cause + ")"
	}
	switch {
	case code == "E745":
		return fail(ReasonInfra, errNotFound, "%s", detail)
	case code == "E747" || cause == "HTTP 401" || cause == "HTTP 403":
		return fail(ReasonTokenRefused, nil, "%s", detail)
	case cause == "HTTP 429":
		return fail(ReasonInfra, errQuota, "%s", detail)
	case cause == "network error" || len(cause) == 8 && cause[:6] == "HTTP 5":
		return fail(ReasonInfra, errUnavailable, "%s", detail)
	}
	return fail(ReasonInfra, nil, "%s", detail)
}

// safeCause admits only the short fixed shapes the infra CLI itself writes.
func safeCause(c string) bool {
	if c == "network error" {
		return true
	}
	if len(c) != 8 || c[:5] != "HTTP " {
		return false
	}
	return c[5] >= '1' && c[5] <= '5' && c[6] >= '0' && c[6] <= '9' && c[7] >= '0' && c[7] <= '9'
}

// run executes one infra verb with the token and returns the data of a success envelope.
func (p *Provider) run(ctx context.Context, token, verb string, args ...string) (json.RawMessage, error) {
	exe, err := p.nself()
	if err != nil {
		return nil, err
	}
	argv := append([]string{exe, "infra", "server", verb, "--json"}, args...)
	argv = append(argv, "--token-env", TokenEnv)
	out, code, rerr := p.cfg.Runner.Run(ctx, argv, []string{TokenEnv + "=" + token}, []string{token})
	if cerr := ctx.Err(); cerr != nil {
		return nil, fail(ReasonInfra, errUnavailable, "infra %s ended with the caller's context: %v", verb, cerr)
	}
	env, ok := parseEnvelope(out)
	switch {
	case ok && env.Error != nil:
		return nil, classify(env, verb)
	case rerr != nil:
		return nil, fail(ReasonInfra, errUnavailable, "infra %s could not run", verb)
	case !ok || code != 0:
		return nil, fail(ReasonAnswer, nil, "infra %s gave no usable answer (exit %d)", verb, code)
	}
	return env.Data, nil
}
