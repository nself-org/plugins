package internal

// ssrf.go — SSRF guard for admin-configurable torrent client URLs.
//
// SECURITY (Security-Always-Free Doctrine): admin-configurable Transmission and
// qBittorrent URLs are written to config at setup time. Without validation an
// operator (or a compromised admin credential) could point the plugin at an
// internal metadata endpoint or another service. This guard resolves the
// destination host and rejects any URL that resolves to a private/internal
// address.
//
// Inputs:    raw admin URL string (TransmissionHost URL or QBittorrent URL).
// Outputs:   nil when safe to use, descriptive error otherwise.
// Constraints: dev opt-in via TORRENT_ALLOW_PRIVATE_URLS=true (never in prod).

import (
	"fmt"
	"net"
	"net/url"
	"os"
)

// adminURLDenyNets lists IP networks that torrent client URLs must not
// resolve to: RFC1918, link-local (169.254/16 — cloud metadata), loopback,
// CGNAT, and reserved ranges.
var adminURLDenyNets []*net.IPNet

func init() {
	cidrs := []string{
		"10.0.0.0/8",         // RFC1918 Class A
		"172.16.0.0/12",      // RFC1918 Class B
		"192.168.0.0/16",     // RFC1918 Class C
		"169.254.0.0/16",     // Link-local / cloud metadata
		"127.0.0.0/8",        // Loopback
		"::1/128",            // IPv6 loopback
		"fc00::/7",           // IPv6 unique local
		"fe80::/10",          // IPv6 link-local
		"0.0.0.0/8",          // Unspecified
		"100.64.0.0/10",      // CGNAT
		"192.0.0.0/24",       // IETF Protocol Assignments
		"198.18.0.0/15",      // Benchmarking
		"198.51.100.0/24",    // TEST-NET-2
		"203.0.113.0/24",     // TEST-NET-3
		"240.0.0.0/4",        // Reserved
		"255.255.255.255/32", // Broadcast
	}
	for _, cidr := range cidrs {
		_, network, err := net.ParseCIDR(cidr)
		if err == nil {
			adminURLDenyNets = append(adminURLDenyNets, network)
		}
	}
}

// allowedAdminPorts is the allowlist of ports for torrent client admin URLs.
// 9091 = Transmission default, 8080 = qBittorrent default, 8989 = Sonarr,
// 7878 = Radarr (common companion tools), 9117 = Jackett.
var allowedAdminPorts = map[string]bool{
	"9091": true,
	"8080": true,
	"8989": true,
	"7878": true,
	"9117": true,
}

// ValidateAdminURL verifies that a torrent client URL is safe to use as an
// admin-configurable endpoint. It:
//  1. Parses the URL and requires http or https scheme.
//  2. Checks the port against the allowlist (or allows empty port for default).
//  3. Resolves the hostname via DNS.
//  4. Rejects any IP in a private/internal network (RFC1918, link-local, loopback).
//
// Returns nil when the URL is safe, or a descriptive error otherwise.
func ValidateAdminURL(rawURL string) error {
	if os.Getenv("TORRENT_ALLOW_PRIVATE_URLS") == "true" {
		// Development opt-in: bypass SSRF guard. Never set in production.
		return nil
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid torrent client URL: %w", err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("torrent client URL scheme %q not allowed (must be http or https)", u.Scheme)
	}

	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("torrent client URL missing host")
	}

	port := u.Port()
	if port != "" && !allowedAdminPorts[port] {
		return fmt.Errorf("torrent client URL port %q not in allowed list (9091, 8080, 8989, 7878, 9117)", port)
	}

	addrs, err := net.LookupHost(host)
	if err != nil {
		return fmt.Errorf("torrent client URL DNS resolution failed for %q: %w", host, err)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("torrent client URL %q resolved to no addresses", host)
	}

	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip == nil {
			return fmt.Errorf("torrent client URL resolved to invalid IP %q", addr)
		}
		for _, deny := range adminURLDenyNets {
			if deny.Contains(ip) {
				return fmt.Errorf("torrent client URL %q resolves to private/internal address %s (SSRF guard)", host, ip)
			}
		}
	}

	return nil
}
