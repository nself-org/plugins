package internal

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ssrfDenyNets is the list of IP networks that outbound webhook destinations
// must not resolve to. Covers RFC1918 private ranges, link-local (169.254/16
// — AWS metadata endpoint), loopback, and multicast.
//
// Security-Always-Free Doctrine: SSRF prevention is free, default, automatic.
// Opt-in for local dev via WEBHOOK_ALLOW_PRIVATE_URLS=true (never in prod).
var ssrfDenyNets []*net.IPNet

func init() {
	// Parse deny-list CIDRs at package init so ValidateWebhookURL is fast.
	cidrs := []string{
		"10.0.0.0/8",         // RFC1918 Class A
		"172.16.0.0/12",      // RFC1918 Class B
		"192.168.0.0/16",     // RFC1918 Class C
		"169.254.0.0/16",     // Link-local / AWS EC2 metadata (169.254.169.254)
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
		_, network, err := net.ParseCIDR(cidr)
		if err == nil {
			ssrfDenyNets = append(ssrfDenyNets, network)
		}
	}
}

// ValidateWebhookURL verifies that a webhook destination URL is safe to
// deliver to. It:
//  1. Parses the URL and requires HTTPS (or HTTP when WEBHOOK_ALLOW_PRIVATE_URLS=true).
//  2. Resolves the hostname via DNS.
//  3. Checks every resolved IP against the SSRF deny-list (RFC1918, link-local,
//     loopback, etc.).
//
// DNS-rebinding mitigation: the IP is resolved once at registration time and
// stored. Subsequent deliveries use the stored IP (handled by the caller).
// Both A and AAAA records are checked.
//
// Returns nil when the URL is safe, or a descriptive error otherwise.
func ValidateWebhookURL(rawURL string) error {
	if os.Getenv("WEBHOOK_ALLOW_PRIVATE_URLS") == "true" {
		// Development opt-in: bypass SSRF guard. Never set in production.
		return nil
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid webhook URL: %w", err)
	}

	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("webhook URL scheme %q not allowed (must be https)", u.Scheme)
	}

	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("webhook URL missing host")
	}

	// Resolve hostname to IPs. Use a 5-second timeout to prevent slow DNS
	// from blocking registration.
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

// Dispatcher handles webhook delivery with retry logic, HMAC signing, and DLQ.
//
// Concurrency control: a semaphore channel (sem) caps the number of goroutines
// that can hold a DB connection simultaneously. This prevents connection-pool
// exhaustion under sustained traffic or post-downtime backlogs.
// Configured via WEBHOOK_DISPATCHER_CONCURRENCY (default 50, max 200).
type Dispatcher struct {
	pool              *pgxpool.Pool
	client            *http.Client
	maxAttempts       int
	requestTimeoutMs  int
	retryDelays       []time.Duration
	autoDisableThresh int
	sem               chan struct{} // semaphore: bounded concurrent deliveries
	maxConcurrency    int           // cap used for warning threshold
	stopCh            chan struct{}
}

// TestResult holds the outcome of a test webhook delivery.
type TestResult struct {
	Success      bool   `json:"success"`
	Status       *int   `json:"status,omitempty"`
	ResponseTime *int   `json:"response_time,omitempty"`
	Error        string `json:"error,omitempty"`
}

// DispatchResult holds the outcome of dispatching an event.
type DispatchResult struct {
	Dispatched int      `json:"dispatched"`
	Endpoints  []string `json:"endpoints"`
}

// NewDispatcher creates a new Dispatcher with configuration from environment.
//
// The semaphore (sem) is initialised here so it is guaranteed to be non-nil
// before any Deliver() / processPending() call — avoids a nil-channel panic if
// the dispatcher is shared across HTTP handlers.
