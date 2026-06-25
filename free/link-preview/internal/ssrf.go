package internal

// ssrf.go — outbound-destination SSRF guard for link-preview URL fetching.
//
// SECURITY (Security-Always-Free Doctrine): the preview URL is user-supplied and
// untrusted. Without validation an attacker can point it at internal services —
// the cloud metadata endpoint (169.254.169.254), RFC1918 hosts, or loopback —
// and exfiltrate credentials or pivot into the network. This guard resolves the
// destination and rejects any URL that resolves to a private/internal address.
// Canonical pattern: free/notify ssrf.go, free/webhooks dispatcher.go.
//
// Inputs:    raw preview URL string.
// Outputs:   nil when safe to fetch, descriptive error otherwise.
// Constraints: dev opt-in via LINK_PREVIEW_ALLOW_PRIVATE_URLS=true (never in prod).

import (
	"fmt"
	"net"
	"net/url"
	"os"
)

// ssrfDenyNets lists IP networks that preview fetch destinations must not
// resolve to: RFC1918, link-local (169.254/16 — cloud metadata), loopback,
// CGNAT, and reserved/test ranges.
var ssrfDenyNets []*net.IPNet

func init() {
	cidrs := []string{
		"10.0.0.0/8",         // RFC1918 Class A
		"172.16.0.0/12",      // RFC1918 Class B
		"192.168.0.0/16",     // RFC1918 Class C
		"169.254.0.0/16",     // Link-local / cloud metadata (169.254.169.254)
		"127.0.0.0/8",        // Loopback
		"::1/128",            // IPv6 loopback
		"fc00::/7",           // IPv6 unique local
		"fe80::/10",          // IPv6 link-local
		"0.0.0.0/8",          // Unspecified / "this" network
		"100.64.0.0/10",      // Shared address space (CGNAT)
		"192.0.0.0/24",       // IETF Protocol Assignments
		"198.18.0.0/15",      // Benchmarking
		"198.51.100.0/24",    // TEST-NET-2
		"203.0.113.0/24",     // TEST-NET-3
		"240.0.0.0/4",        // Reserved
		"255.255.255.255/32", // Broadcast
	}
	for _, cidr := range cidrs {
		if _, network, err := net.ParseCIDR(cidr); err == nil {
			ssrfDenyNets = append(ssrfDenyNets, network)
		}
	}
}

// validatePreviewURL verifies a preview-fetch URL is safe to request: it must be
// http/https with a host, and every IP the host resolves to must be outside the
// SSRF deny-list.
func validatePreviewURL(rawURL string) error {
	if os.Getenv("LINK_PREVIEW_ALLOW_PRIVATE_URLS") == "true" {
		return nil // dev opt-in; never set in production
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid preview URL: %w", err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("preview URL scheme %q not allowed", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("preview URL missing host")
	}

	addrs, err := net.LookupHost(host)
	if err != nil {
		return fmt.Errorf("preview URL DNS resolution failed for %q: %w", host, err)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("preview URL %q resolved to no addresses", host)
	}
	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip == nil {
			return fmt.Errorf("preview URL resolved to invalid IP %q", addr)
		}
		for _, deny := range ssrfDenyNets {
			if deny.Contains(ip) {
				return fmt.Errorf("preview destination %q resolves to private/internal address %s (SSRF guard)", host, ip)
			}
		}
	}
	return nil
}
