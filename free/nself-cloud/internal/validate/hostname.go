// Package validate provides input validation helpers for nself-cloud.
package validate

import (
	"errors"
	"regexp"
	"strings"
)

// rfc1123Label matches a single DNS label per RFC 1123:
// starts and ends with alnum, middle may contain hyphens, max 63 chars.
var rfc1123Label = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?$`)

// reserved hostnames that must never be bound to a tenant domain.
var reservedHostnames = map[string]struct{}{
	"localhost":      {},
	"internal":       {},
	"local":          {},
	"invalid":        {},
	"example":        {},
	"test":           {},
	"home":           {},
	"broadcasthost":  {},
	"ip6-localhost":  {},
	"ip6-loopback":   {},
	"ip6-localnet":   {},
	"ip6-mcastprefix": {},
	"ip6-allnodes":   {},
	"ip6-allrouters": {},
	"ip6-allhosts":   {},
}

// ValidateHostname validates hostname according to RFC 1123.
// It rejects:
//   - empty strings
//   - hostnames longer than 253 characters
//   - labels longer than 63 characters
//   - invalid characters (any char not matching the label regex)
//   - path-traversal or injection attempts (e.g. "../../../etc/passwd")
//   - HTML/script injection (e.g. "<script>")
//   - reserved hostnames (localhost, internal, etc.)
//
// A single-label hostname (e.g. "myhost") is accepted only if it passes all
// other checks; callers that require FQDN must enforce that separately.
func ValidateHostname(input string) error {
	if input == "" {
		return errors.New("hostname must not be empty")
	}

	// Reject anything containing path separators or null bytes — fast injection guard.
	if strings.ContainsAny(input, "/\\<>\x00") {
		return errors.New("hostname contains invalid characters")
	}

	// Total length limit (253 per RFC 1035 §3.1, consistent with RFC 1123).
	if len(input) > 253 {
		return errors.New("hostname exceeds maximum length of 253 characters")
	}

	// Strip a single trailing dot (FQDN notation) before label checks.
	stripped := strings.TrimSuffix(input, ".")
	if stripped == "" {
		return errors.New("hostname must not be empty after stripping trailing dot")
	}

	labels := strings.Split(stripped, ".")
	for _, label := range labels {
		if label == "" {
			return errors.New("hostname contains empty label (consecutive dots)")
		}
		if len(label) == 1 && !isAlnum(rune(label[0])) {
			return errors.New("single-character label must be alphanumeric")
		}
		if !rfc1123Label.MatchString(label) {
			return errors.New("hostname label '" + label + "' is invalid: must start and end with alphanumeric and contain only [a-zA-Z0-9-]")
		}
	}

	// Reject reserved hostnames.
	// Single-label hostnames (no dot) are checked against the full name.
	// Multi-label hostnames are NOT rejected based on first label alone —
	// e.g. "test.example.com" is valid even though "test" and "example" are reserved
	// bare names. We only block exact matches on the full stripped hostname.
	lower := strings.ToLower(stripped)
	if _, ok := reservedHostnames[lower]; ok {
		return errors.New("hostname '" + input + "' is reserved and cannot be bound")
	}

	return nil
}

func isAlnum(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}
