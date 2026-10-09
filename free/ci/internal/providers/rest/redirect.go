package rest

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strings"
)

func hostMatch(host, permitted string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	permitted = strings.TrimSuffix(strings.ToLower(permitted), ".")
	if strings.HasPrefix(permitted, "*.") {
		permitted = permitted[2:]
		return strings.HasSuffix(host, "."+permitted)
	}
	return host == permitted
}

func sameAuthority(target *url.URL, rawBase string) bool {
	base, err := url.Parse(rawBase)
	if err != nil || target == nil || target.Scheme != "https" || base.Scheme != "https" {
		return false
	}
	port := func(u *url.URL) string {
		if u.Port() != "" {
			return u.Port()
		}
		return "443"
	}
	return strings.EqualFold(target.Hostname(), base.Hostname()) && port(target) == port(base)
}

func (c *Client) allowedURL(u *url.URL, includeBase bool) bool {
	if u == nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || net.ParseIP(u.Hostname()) != nil {
		return false
	}
	if includeBase && strings.EqualFold(u.Hostname(), c.baseHost()) {
		return true
	}
	for _, host := range c.RedirectHosts {
		if hostMatch(u.Hostname(), host) {
			return true
		}
	}
	return false
}

func (c *Client) checkRedirect(req *http.Request, via []*http.Request) error {
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		return config("redirect.method")
	}
	if len(via) >= 4 || !c.allowedURL(req.URL, false) {
		return config("redirect.host")
	}
	// A redirect never inherits permission to dial the private base, even on the same host.
	*req = *req.WithContext(context.WithValue(req.Context(), privateBaseKey{}, false))
	if len(via) > 0 && !strings.EqualFold(req.URL.Host, via[0].URL.Host) {
		req.Header.Del("Authorization")
	}
	return nil
}
