//go:build !linux

package netscope

import "net/netip"

// routePrefixes has no portable rootless source off Linux; the interface prefixes stand alone there, and
// the sidecar needs Linux anyway (the probe reports the scopes unavailable elsewhere).
func routePrefixes() ([]netip.Prefix, error) { return nil, nil }
