package deployguard

// Purpose: one canonical spelling of a host, so every way of writing a listed host meets the same key.
// Inputs:  any host string an inventory, ssh config, agent or operator may carry.
// Outputs: Canon (the key) and Variants (the keys a string may stand for).
// Constraints: no network; the lenient reader below accepts every spelling remote.ParseHostSpec does and the
//          malformed ones too (a boundary must not give up on garbage); case, trailing dots, brackets, user@ and :port never change the key; IPv4-in-IPv6
//          and zone-id forms map to the plain address; inet_aton shorthand (decimal, hex, octal, short
//          dotted) is read the way ssh and getaddrinfo read it; a string that is not a host maps to "".

import (
	"net/netip"
	"strconv"
	"strings"
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
	if i := strings.LastIndex(s, "@"); i >= 0 {
		s = s[i+1:]
	}
	switch {
	case strings.HasPrefix(s, "["):
		inner, _, _ := strings.Cut(s[1:], "]")
		s = inner
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
	h, _, _ = strings.Cut(h, "%")
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
	// A part may fill the bytes left in the address: the last of n parts has 5-n bytes (inet_aton).
	limits := [4]uint64{1 << 32, 1 << 24, 1 << 16, 1 << 8}
	var v uint64
	last := len(nums) - 1
	for i := 0; i < last; i++ {
		if nums[i] > 255 {
			return netip.Addr{}, false
		}
		v |= nums[i] << (8 * (3 - uint(i)))
	}
	if nums[last] >= limits[last] {
		return netip.Addr{}, false
	}
	v |= nums[last]
	return netip.AddrFrom4([4]byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}), true
}

// NormalizeFingerprint returns the SHA256:<base64> form of a host-key fingerprint, keeping the case of
// the digest (base64 is case sensitive); other text maps to "".
func NormalizeFingerprint(raw string) string {
	s := strings.TrimSpace(raw)
	if rest, ok := strings.CutPrefix(s, "SHA256:"); ok {
		s = rest
	} else if rest, ok = strings.CutPrefix(s, "sha256:"); ok {
		s = rest
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
