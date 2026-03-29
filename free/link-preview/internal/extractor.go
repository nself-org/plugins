package internal

import (
	"fmt"
	"io"
	"net/http"
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

// ExtractMetadata fetches a URL and extracts metadata from the HTML.
func ExtractMetadata(targetURL string) (*Metadata, error) {
	client := &http.Client{
		Timeout: fetchTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
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
