package deployguard

// Purpose: one canonical spelling of a host, so every way of writing a listed host meets the same key.
// Inputs:  any host string an inventory, ssh config, agent or operator may carry.
// Outputs: Canon (the key) and Variants (the keys a string may stand for).
// Constraints: no network; case, trailing dots, brackets, user@ and :port never change the key; IPv4-in-IPv6
//          and zone-id forms map to the plain address; inet_aton shorthand (decimal, hex, octal, short
//          dotted) is read the way ssh and getaddrinfo read it; a string that is not a host maps to "".

import (
	"net/netip"
	"strconv"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/remote"
)

// Canon returns the canonical key of one host string, or "" when it names nothing.
func Canon(raw string) string {
	h := bareHost(raw)
	if h == "" {
		return ""
	}
	if a, ok := parseAddr(h); ok {
		return a.String()
	}
	return h
}

// bareHost strips user, brackets, port and legacy path and lower-cases the rest.
func bareHost(raw string) string {
	s := strings.TrimSpace(raw)
	if spec, err := remote.ParseHostSpec(s); err == nil && spec.Host != "" {
		return tidy(spec.Host)
	}
	if i := strings.LastIndex(s, "@"); i >= 0 {
		s = s[i+1:]
	}
	switch {
	case strings.HasPrefix(s, "["):
		if j := strings.Index(s, "]"); j > 0 {
			s = s[1:j]
		} else {
			s = strings.TrimPrefix(s, "[")
		}
	case strings.Count(s, ":") == 1:
		s, _, _ = strings.Cut(s, ":")
	}
	return tidy(s)
}

func tidy(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	for strings.HasSuffix(s, ".") {
		s = strings.TrimSuffix(s, ".")
	}
	return s
}

func parseAddr(h string) (netip.Addr, bool) {
	if i := strings.Index(h, "%"); i >= 0 {
		h = h[:i]
	}
	if a, err := netip.ParseAddr(h); err == nil {
		return a.Unmap(), true
	}
	if a, ok := inetAton(h); ok {
		return a, true
	}
	return netip.Addr{}, false
}

// inetAton reads the 1 to 4 part IPv4 forms with decimal, 0x hex and leading-zero octal parts.
func inetAton(h string) (netip.Addr, bool) {
	parts := strings.Split(h, ".")
	if len(parts) > 4 {
		return netip.Addr{}, false
	}
	nums := make([]uint64, len(parts))
	for i, p := range parts {
		if p == "" {
			return netip.Addr{}, false
		}
		base := 10
		switch {
		case strings.HasPrefix(p, "0x") || strings.HasPrefix(p, "0X"):
			base, p = 16, p[2:]
		case len(p) > 1 && p[0] == '0':
			base, p = 8, p[1:]
		}
		n, err := strconv.ParseUint(p, base, 32)
		if err != nil {
			return netip.Addr{}, false
		}
		nums[i] = n
	}
	var v uint64
	last := len(nums) - 1
	for i := 0; i < last; i++ {
		if nums[i] > 255 {
			return netip.Addr{}, false
		}
		v |= nums[i] << (8 * (3 - uint(i)))
	}
	if nums[last] >= 1<<(8*(4-uint(last))) {
		return netip.Addr{}, false
	}
	v |= nums[last]
	return netip.AddrFrom4([4]byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}), true
}

// NormalizeFingerprint returns the SHA256:<base64> form of a host-key fingerprint, keeping the case of
// the digest (base64 is case sensitive); other text maps to "".
func NormalizeFingerprint(raw string) string {
	s := strings.TrimSpace(raw)
	if len(s) > 7 && strings.EqualFold(s[:7], "sha256:") {
		s = s[7:]
	}
	s = strings.TrimRight(s, "=")
	if len(s) != 43 || strings.ContainsAny(s, " \t:") {
		return ""
	}
	return "SHA256:" + s
}

// NormalizeMachineID lower-cases a machine id and drops dashes; ids are 32 hex digits.
func NormalizeMachineID(raw string) string {
	s := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(raw), "-", ""))
	if s == "" {
		return ""
	}
	return s
}
