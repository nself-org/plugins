package attest

import (
	"net/netip"
	"regexp"
	"strconv"
	"strings"
)

var legacyIPv4 = regexp.MustCompile(`\b(?:0[xX][0-9a-fA-F]+|[0-9]+)(?:\.(?:0[xX][0-9a-fA-F]+|[0-9]+)){0,3}\b`)

// inet-style IPv4 permits one to four components, including hex and octal.
func parseLegacyIPv4(s string) (netip.Addr, bool) {
	parts := strings.Split(s, ".")
	if len(parts) > 4 {
		return netip.Addr{}, false
	}
	var value uint64
	for i, part := range parts {
		bits := 8
		if i == len(parts)-1 {
			bits = 8 * (5 - len(parts))
		}
		base := 10
		if strings.HasPrefix(part, "0x") || strings.HasPrefix(part, "0X") || len(part) > 1 && part[0] == '0' {
			base = 0
		}
		n, err := strconv.ParseUint(part, base, bits)
		if err != nil {
			return netip.Addr{}, false
		}
		value = value<<bits | n
	}
	return netip.AddrFrom4([4]byte{byte(value >> 24), byte(value >> 16), byte(value >> 8), byte(value)}), true
}

func (p Profile) scrubLegacyIPv4(s string) string {
	return legacyIPv4.ReplaceAllStringFunc(s, func(m string) string {
		if a, ok := parseLegacyIPv4(m); ok && (privateAddr(a) || p.knownAddr(a)) {
			return redactedIP
		}
		return m
	})
}
