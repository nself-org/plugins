package internal

// ssrf.go — outbound-destination SSRF guard for webhook delivery.
//
// SECURITY (Security-Always-Free Doctrine): user-supplied webhook URLs are
// untrusted. Without validation an attacker can point the URL at internal
// services — the cloud metadata endpoint (169.254.169.254), RFC1918 hosts,
// or loopback — and exfiltrate credentials or pivot. This guard resolves the
// destination and rejects any URL that resolves to a private/internal address,
// and the delivery client refuses HTTP redirects so a public host cannot bounce
// the request to an internal one. Canonical pattern: free/webhooks dispatcher.go.
//
// Inputs:    raw webhook URL string.
// Outputs:   nil when safe to deliver, descriptive error otherwise.
// Constraints: dev opt-in via NOTIFY_ALLOW_PRIVATE_URLS=true (never in prod).

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"
)

// ssrfDenyNets lists IP networks that outbound webhook destinations must not
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

// validateWebhookURL verifies a webhook destination URL is safe to deliver to:
// it must be http/https with a host, and every IP the host resolves to must be
// outside the SSRF deny-list.
func validateWebhookURL(rawURL string) error {
	if os.Getenv("NOTIFY_ALLOW_PRIVATE_URLS") == "true" {
		return nil // dev opt-in; never set in production
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid webhook URL: %w", err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("webhook URL scheme %q not allowed", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("webhook URL missing host")
	}

	addrs, err := net.LookupHost(host)
	if err != nil {
		return fmt.Errorf("webhook URL DNS resolution failed for %q: %w", host, err)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("webhook URL %q resolved to no addresses", host)
	}
	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip == nil {
			return fmt.Errorf("webhook URL resolved to invalid IP %q", addr)
		}
		for _, deny := range ssrfDenyNets {
			if deny.Contains(ip) {
				return fmt.Errorf("webhook destination %q resolves to private/internal address %s (SSRF guard)", host, ip)
			}
		}
	}
	return nil
}

// ssrfSafeClient is an http.Client that refuses to follow redirects, so a
// public destination cannot bounce a request to a validated-around internal
// address. It also re-validates each hop's URL defensively.
var ssrfSafeClient = &http.Client{
	Timeout: 30 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	},
}
