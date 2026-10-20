package webhook

// Purpose: the two checks that authenticate a GitLab delivery (Epic D11, X3): the Standard Webhooks
// signature of a signing token, and the bare X-Gitlab-Token compare.
// Inputs:  the secret value read from the environment, the webhook-id / webhook-timestamp /
//          webhook-signature headers, the raw body and the clock.
// Outputs: nil, or an E706 with reason signature, token or timestamp.
// Constraints: the key is the base64 decoding of the secret after its "whsec_" prefix; the signed
//          content is id "." timestamp "." body; the header is a space separated list of "v1,<base64>"
//          entries and every entry is compared with hmac.Equal (none is skipped after a match, so the
//          time taken does not say which entry matched); an empty or malformed secret never verifies;
//          the MAC is checked before the timestamp, so a sender without the key learns only
//          "signature"; a timestamp more than 5 minutes from the clock (either way) is refused; the
//          bare token is compared in constant time over fixed length digests and an empty token never
//          matches. Headers are length bounded before any hashing.

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"strconv"
	"strings"
	"time"
)

const (
	secretPrefix  = "whsec_"
	sigVersion    = "v1,"
	maxSkew       = 5 * time.Minute
	maxIDLen      = 128
	maxSigHeader  = 2048
	maxSigEntries = 8
)

// decodeKey returns the HMAC key of a "whsec_" secret.
func decodeKey(secret string) ([]byte, bool) {
	b64, ok := strings.CutPrefix(secret, secretPrefix)
	if !ok || b64 == "" {
		return nil, false
	}
	key, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		key, err = base64.RawStdEncoding.DecodeString(b64)
	}
	if err != nil || len(key) == 0 {
		return nil, false
	}
	return key, true
}

// signedContent is what GitLab signs: id.timestamp.body.
func signedContent(id, ts string, body []byte) []byte {
	out := make([]byte, 0, len(id)+len(ts)+len(body)+2)
	out = append(out, id...)
	out = append(out, '.')
	out = append(out, ts...)
	out = append(out, '.')
	return append(out, body...)
}

// verifySigned checks a Standard Webhooks delivery against the signing secret.
func verifySigned(secret, id, ts, header string, body []byte, now time.Time) error {
	key, ok := decodeKey(secret)
	if !ok || id == "" || len(id) > maxIDLen || ts == "" || header == "" || len(header) > maxSigHeader {
		return errSignature()
	}
	m := hmac.New(sha256.New, key)
	m.Write(signedContent(id, ts, body))
	want := m.Sum(nil)
	entries := strings.Fields(header)
	if len(entries) > maxSigEntries {
		entries = entries[:maxSigEntries]
	}
	matched := false
	for _, e := range entries {
		enc, isV1 := strings.CutPrefix(e, sigVersion)
		if !isV1 {
			continue
		}
		got, err := base64.StdEncoding.DecodeString(enc)
		if err != nil {
			continue
		}
		if hmac.Equal(got, want) {
			matched = true // keep looping: the time does not depend on the position of the match
		}
	}
	if !matched {
		return errSignature()
	}
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return errTimestamp()
	}
	age := now.Sub(time.Unix(sec, 0))
	if age > maxSkew || age < -maxSkew {
		return errTimestamp()
	}
	return nil
}

// verifyToken compares the X-Gitlab-Token header with the configured token in constant time.
func verifyToken(configured, header string) error {
	if configured == "" || header == "" {
		return errToken()
	}
	a, b := sha256.Sum256([]byte(configured)), sha256.Sum256([]byte(header))
	if subtle.ConstantTimeCompare(a[:], b[:]) != 1 {
		return errToken()
	}
	return nil
}
