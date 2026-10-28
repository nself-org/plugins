package netscope

// Purpose: validate a route destination read from the kernel.
// Inputs: IPv4/IPv6 network bytes and prefix length. Outputs: masked prefix.
// Constraints: malformed prefix lengths are errors, never partial deny sets.

import (
	"fmt"
	"net/netip"
)

func routePrefix(dst []byte, bits int) (netip.Prefix, error) {
	addr, ok := netip.AddrFromSlice(dst)
	if !ok || bits <= 0 || bits > addr.BitLen() {
		return netip.Prefix{}, fmt.Errorf("invalid route destination or prefix length")
	}
	return netip.PrefixFrom(addr, bits).Masked(), nil
}
