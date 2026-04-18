package sdk

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Webhook signing standard — S18-T08.
//
// The nSelf webhooks + cron + jobs plugins all sign outbound POSTs the same
// way so receivers can reuse one verifier. Format:
//
//   X-Webhook-Signature: t=<unix_ts>,v1=<hex>
//   X-Webhook-Signature-Version: v1
//
// Where <hex> = hex(HMAC-SHA256(secret, "<unix_ts>.<raw_body>")).
//
// This file implements the receiver side: parse, constant-time compare,
// replay-window guard. Integrators can drop this into their own Go service
// (copy-paste is fine — it only depends on the stdlib) or import the SDK.

// ErrMissingSignature is returned when the request carries no signature header.
var ErrMissingSignature = errors.New("missing X-Webhook-Signature header")

// ErrMalformedSignature is returned when the header value cannot be parsed.
var ErrMalformedSignature = errors.New("malformed X-Webhook-Signature header")

// ErrSignatureExpired is returned when the signature timestamp is outside the
// allowed replay window.
var ErrSignatureExpired = errors.New("signature timestamp outside tolerance window")

// ErrSignatureMismatch is returned when the HMAC does not match.
var ErrSignatureMismatch = errors.New("signature does not match")

// ErrUnsupportedVersion is returned when the signature version is not v1.
var ErrUnsupportedVersion = errors.New("unsupported signature version")

// DefaultTolerance is the default replay-window for webhook signatures.
// Matches Stripe's default (5 minutes) which is long enough for clock skew
// but short enough to frustrate replay attacks.
const DefaultTolerance = 5 * time.Minute

// VerifyWebhookSignature validates a signed webhook payload.
//
// Parameters:
//   header    — the raw X-Webhook-Signature header value ("t=<ts>,v1=<hex>")
//   body      — the raw request body bytes (NOT parsed/re-marshaled)
//   secret    — the shared signing secret
//   tolerance — replay window; use DefaultTolerance for the nSelf default
//
// Returns nil on successful verification; a sentinel error otherwise.
//
// Pass tolerance=0 to disable the replay check (only recommended in tests).
func VerifyWebhookSignature(header string, body []byte, secret string, tolerance time.Duration) error {
	if header == "" {
		return ErrMissingSignature
	}
	if secret == "" {
		return errors.New("signing secret is empty — refusing to verify")
	}

	ts, sig, err := parseSignatureHeader(header)
	if err != nil {
		return err
	}

	if tolerance > 0 {
		age := time.Since(time.Unix(ts, 0))
		if age < 0 {
			age = -age
		}
		if age > tolerance {
			return fmt.Errorf("%w: age=%s tolerance=%s", ErrSignatureExpired, age, tolerance)
		}
	}

	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%d.%s", ts, body)
	expected := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(expected), []byte(sig)) {
		return ErrSignatureMismatch
	}
	return nil
}

// VerifyWebhookRequest is a convenience wrapper around VerifyWebhookSignature
// that reads the body off an *http.Request and refills it so downstream
// handlers can re-read it. Returns the (already-consumed) body bytes and any
// verification error.
func VerifyWebhookRequest(r *http.Request, secret string, tolerance time.Duration) ([]byte, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	_ = r.Body.Close()
	header := r.Header.Get("X-Webhook-Signature")
	if err := VerifyWebhookSignature(header, body, secret, tolerance); err != nil {
		return body, err
	}
	return body, nil
}

// SignWebhookPayload produces the "t=<ts>,v1=<hex>" header value. Senders
// should set the result as X-Webhook-Signature and also set
// X-Webhook-Signature-Version to "v1".
func SignWebhookPayload(body []byte, secret string, ts time.Time) string {
	unix := ts.Unix()
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%d.%s", unix, body)
	return fmt.Sprintf("t=%d,v1=%s", unix, hex.EncodeToString(mac.Sum(nil)))
}

// parseSignatureHeader splits "t=<ts>,v1=<hex>" into (ts, hex).
// Unknown version tags are rejected with ErrUnsupportedVersion so new
// versions are opt-in, never silently accepted.
func parseSignatureHeader(header string) (int64, string, error) {
	var ts int64 = -1
	var sig string
	for _, part := range strings.Split(header, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			return 0, "", ErrMalformedSignature
		}
		switch kv[0] {
		case "t":
			n, err := strconv.ParseInt(kv[1], 10, 64)
			if err != nil {
				return 0, "", ErrMalformedSignature
			}
			ts = n
		case "v1":
			sig = kv[1]
		default:
			// Unknown scheme (v2, v3, …) — surface explicitly rather than
			// silently accepting an unverified payload.
			if strings.HasPrefix(kv[0], "v") {
				return 0, "", ErrUnsupportedVersion
			}
		}
	}
	if ts < 0 || sig == "" {
		return 0, "", ErrMalformedSignature
	}
	return ts, sig, nil
}
