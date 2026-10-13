package httpx

// ssrf.go — shared outbound-destination SSRF guard.
//
// SECURITY (Security-Always-Free Doctrine): user-supplied webhook/callback
// URLs are untrusted. Without validation an attacker can point a URL at
// internal services — cloud metadata endpoints (169.254.169.254), RFC1918
// hosts, or loopback — and exfiltrate credentials or pivot to other services.
//
// This guard resolves the destination hostname and rejects any URL whose IPs
// fall within private/internal CIDR blocks. It is the canonical SSRF guard
// shared by all nSelf plugins. Plugin-local copies are deprecated and should
// delegate here.
//
// Inputs:    raw URL string.
// Outputs:   nil when safe to deliver, descriptive error otherwise.
// Constraints: dev opt-in via NSELF_ALLOW_PRIVATE_URLS=true (never in prod).

import (
	"fmt"
	"net"
	"net/url"
	"os"
)

// ssrfDenyNets is the list of IP networks that outbound webhook/callback
// destinations must not resolve to. Covers RFC1918, link-local (cloud
// metadata), loopback, IPv6 private ranges, and reserved/test ranges.
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

// ValidateOutboundURL verifies a user-supplied URL is safe to make an outbound
// HTTP request to. It:
//  1. Parses the URL and requires http or https scheme.
//  2. Resolves the hostname via DNS (both A and AAAA records).
//  3. Checks every resolved IP against the SSRF deny-list.
//
// Returns nil when the URL is safe, or a descriptive error otherwise.
//
// DNS-rebinding mitigation: callers should store the validated IP and connect
// directly to it rather than re-resolving at send time.
func ValidateOutboundURL(rawURL string) error {
	if os.Getenv("NSELF_ALLOW_PRIVATE_URLS") == "true" {
		// Development opt-in: bypass SSRF guard. Never set in production.
		return nil
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("URL scheme %q not allowed (must be http or https)", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("URL missing host")
	}

	addrs, err := net.LookupHost(host)
	if err != nil {
		return fmt.Errorf("DNS resolution failed for %q: %w", host, err)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("URL %q resolved to no addresses", host)
	}
	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip == nil {
			return fmt.Errorf("URL resolved to invalid IP %q", addr)
		}
		for _, deny := range ssrfDenyNets {
			if deny.Contains(ip) {
				return fmt.Errorf("destination %q resolves to private/internal address %s (SSRF guard)", host, ip)
			}
		}
	}
	return nil
}
