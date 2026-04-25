package internal

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const (
	fetchTimeout = 10 * time.Second
	userAgent    = "Mozilla/5.0 (compatible; nSelfBot/1.0; +https://nself.org)"
	maxBodySize  = 5 * 1024 * 1024 // 5 MB
)

// Metadata holds extracted Open Graph, Twitter Card, and HTML metadata.
type Metadata struct {
	Title       string
	Description string
	Image       string
	SiteName    string
	Type        string
}

// checkSSRF validates a URL against SSRF risks before fetching.
// Blocks: non-http/https schemes, private/loopback/link-local IPs (RFC1918,
// 169.254/16), internal hostnames (*.internal, *.local), and decimal IP notation.
func checkSSRF(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	switch parsed.Scheme {
	case "http", "https":
		// ok
	default:
		return fmt.Errorf("scheme '%s' not allowed (only http/https)", parsed.Scheme)
	}

	host := parsed.Hostname()

	// Block internal hostname suffixes
	if strings.HasSuffix(host, ".internal") || strings.HasSuffix(host, ".local") {
		return fmt.Errorf("host '%s' is an internal hostname", host)
	}

	if ip := net.ParseIP(host); ip != nil {
		if isPrivateIP(ip) {
			return fmt.Errorf("IP '%s' is in a private/loopback/link-local range", ip)
		}
	}

	// DNS rebinding prevention: resolve and check all resulting IPs.
	if net.ParseIP(host) == nil {
		addrs, err := net.LookupHost(host)
		if err == nil {
			for _, addr := range addrs {
				if ip := net.ParseIP(addr); ip != nil && isPrivateIP(ip) {
					return fmt.Errorf("hostname '%s' resolves to blocked IP %s", host, addr)
				}
			}
		}
	}

	return nil
}

// isPrivateIP returns true for loopback, private, link-local, and unspecified addresses.
func isPrivateIP(ip net.IP) bool {
	if ip.IsUnspecified() || ip.IsLoopback() {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		// 10.0.0.0/8
		if ip4[0] == 10 {
			return true
		}
		// 172.16.0.0/12
		if ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 {
			return true
		}
		// 192.168.0.0/16
		if ip4[0] == 192 && ip4[1] == 168 {
			return true
		}
		// 169.254.0.0/16 (link-local / cloud metadata)
		if ip4[0] == 169 && ip4[1] == 254 {
			return true
		}
		return false
	}
	// IPv6 link-local fe80::/10
	if ip[0] == 0xfe && (ip[1]&0xc0) == 0x80 {
		return true
	}
	// IPv6 ULA fc00::/7
	if (ip[0] & 0xfe) == 0xfc {
		return true
	}
	return false
}

// ExtractMetadata fetches a URL and extracts metadata from the HTML.
// Returns an error if the URL is blocked by SSRF protection.
func ExtractMetadata(targetURL string) (*Metadata, error) {
	if err := checkSSRF(targetURL); err != nil {
		return nil, fmt.Errorf("SSRF check: %w", err)
	}

	client := &http.Client{
		Timeout: fetchTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			// Validate redirect destinations too
			if err := checkSSRF(req.URL.String()); err != nil {
				return fmt.Errorf("redirect blocked: %w", err)
			}
			return nil
		},
	}

	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch url: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if ct != "" && !strings.Contains(ct, "text/html") && !strings.Contains(ct, "application/xhtml+xml") {
		return nil, fmt.Errorf("unsupported content type: %s", ct)
	}

	limited := io.LimitReader(resp.Body, maxBodySize)
	return parseHTML(limited)
}

// parseHTML parses an HTML document and extracts OG, Twitter Card, and fallback metadata.
func parseHTML(r io.Reader) (*Metadata, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}

	m := &Metadata{}

	// Track fallback values separately.
	var htmlTitle string
	var metaDescription string
	var twitterTitle string
	var twitterDescription string
	var twitterImage string

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "title":
				if n.FirstChild != nil && n.FirstChild.Type == html.TextNode {
					htmlTitle = strings.TrimSpace(n.FirstChild.Data)
				}
			case "meta":
				handleMeta(n, m, &metaDescription, &twitterTitle, &twitterDescription, &twitterImage)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	// Apply fallback priority: OG > Twitter > HTML/meta
	if m.Title == "" {
		if twitterTitle != "" {
			m.Title = twitterTitle
		} else {
			m.Title = htmlTitle
		}
	}
	if m.Description == "" {
		if twitterDescription != "" {
			m.Description = twitterDescription
		} else {
			m.Description = metaDescription
		}
	}
	if m.Image == "" {
		m.Image = twitterImage
	}

	return m, nil
}

// handleMeta inspects a <meta> element for OG, Twitter Card, and standard tags.
func handleMeta(n *html.Node, m *Metadata, metaDesc, twTitle, twDesc, twImage *string) {
	attrs := attrMap(n)

	// Open Graph (property="og:...")
	if prop, ok := attrs["property"]; ok {
		content := attrs["content"]
		switch prop {
		case "og:title":
			m.Title = content
		case "og:description":
			m.Description = content
		case "og:image":
			m.Image = content
		case "og:site_name":
			m.SiteName = content
		case "og:type":
			m.Type = content
		}
	}

	// Twitter Cards (name="twitter:...")
	if name, ok := attrs["name"]; ok {
		content := attrs["content"]
		switch name {
		case "twitter:title":
			*twTitle = content
		case "twitter:description":
			*twDesc = content
		case "twitter:image":
			*twImage = content
		case "description":
			*metaDesc = content
		}
	}
}

// attrMap returns a map of attribute key to value for an HTML node.
func attrMap(n *html.Node) map[string]string {
	m := make(map[string]string, len(n.Attr))
	for _, a := range n.Attr {
		m[a.Key] = a.Val
	}
	return m
}
