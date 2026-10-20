//go:build security

package redteam

// Purpose: TM-33 red team, SSRF half. The REST funnel (providers/rest) is pointed at loopback
// targets through a user-supplied forge base URL, a redirect, a download URL and hostile path
// parameters. A canary server on loopback counts every request it receives.
// Inputs:  two loopback TLS servers (the "forge" and the canary).
// Outputs: pass = the canary sees nothing and path parameters stay one escaped segment (HELD).
// Constraints: loopback only; AllowPrivateBase is true only for the first hop where the test needs a
//          reachable fake forge.

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
)

type dCanary struct {
	srv  *httptest.Server
	hits atomic.Int32
	mu   sync.Mutex
	uris []string
	auth []string
}

func newDCanary(t *testing.T, h http.HandlerFunc) *dCanary {
	t.Helper()
	c := &dCanary{}
	c.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.hits.Add(1)
		c.mu.Lock()
		c.uris, c.auth = append(c.uris, r.RequestURI), append(c.auth, r.Header.Get("Authorization"))
		c.mu.Unlock()
		if h != nil {
			h(w, r)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(c.srv.Close)
	return c
}

func (c *dCanary) port() string { u, _ := url.Parse(c.srv.URL); return u.Port() }

func (c *dCanary) tls() *rest.Client {
	return &rest.Client{BaseURL: c.srv.URL, Provider: "github", TLSConfig: c.srv.Client().Transport.(*http.Transport).TLSClientConfig}
}

// D13: a user-supplied base URL that names loopback is refused before any request leaves, however the
// host is spelled, unless the operator turned AllowPrivateBase on.
func TestD13_LoopbackBaseURLRefused(t *testing.T) {
	canary := newDCanary(t, nil)
	for _, host := range []string{"127.0.0.1", "localhost", "LOCALHOST.", "[::1]", "[::ffff:127.0.0.1]"} {
		c := canary.tls()
		c.BaseURL = "https://" + host + ":" + canary.port()
		_, err := c.Do(context.Background(), "GET", "/repos/{o}/{r}", map[string]string{"o": "a", "r": "b"}, nil, nil)
		var pe *providers.Error
		if !errors.As(err, &pe) {
			t.Errorf("%s: want a provider error, got %v", host, err)
		} else if pe.Reason != "ssrf.private" && pe.Reason != "base.url" {
			t.Logf("%s: refused with reason %q", host, pe.Reason)
		}
	}
	if n := canary.hits.Load(); n != 0 {
		t.Fatalf("canary received %d request(s) through a loopback base URL", n)
	}
	// positive control: the same server answers when the operator allows a private base
	c := canary.tls()
	c.AllowPrivateBase = true
	if _, err := c.Do(context.Background(), "GET", "/ping", nil, nil, nil); err != nil || canary.hits.Load() != 1 {
		t.Fatalf("positive control failed: err=%v hits=%d", err, canary.hits.Load())
	}
}

// D14: a forge that redirects into loopback (by IP literal, or by a host name the operator listed as a
// redirect host) never gets the funnel to dial it, and the first hop's credential is not forwarded.
func TestD14_RedirectIntoLoopbackRefused(t *testing.T) {
	canary := newDCanary(t, nil)
	for name, target := range map[string]string{
		"ip literal":     "https://127.0.0.1:" + canary.port() + "/steal",
		"listed host":    "https://localhost:" + canary.port() + "/steal",
		"http downgrade": "http://localhost:" + canary.port() + "/steal",
		"userinfo":       "https://forge.example@localhost:" + canary.port() + "/steal",
	} {
		target := target
		forge := newDCanary(t, func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target, http.StatusFound) })
		c := forge.tls()
		c.AllowPrivateBase = true
		c.RedirectHosts = []string{"localhost", "*.localhost"}
		c.Auth = func(context.Context) (string, error) { return "synthetic-token", nil }
		_, err := c.Do(context.Background(), "GET", "/start", nil, nil, nil)
		if err == nil {
			t.Errorf("%s: redirect into loopback succeeded", name)
		}
		if n := canary.hits.Load(); n != 0 {
			t.Fatalf("%s: canary received %d request(s) through a redirect", name, n)
		}
	}
}

// D15: a download URL on the base host but another port, or on loopback, never dials the private
// address (downloads never inherit the private-base allowance).
func TestD15_DownloadNeverDialsPrivate(t *testing.T) {
	canary := newDCanary(t, nil)
	forge := newDCanary(t, nil)
	c := forge.tls()
	c.AllowPrivateBase = true
	c.Auth = func(context.Context) (string, error) { return "synthetic-token", nil }
	var sink bytes.Buffer
	for _, raw := range []string{forge.srv.URL + "/artifact", "https://127.0.0.1:" + canary.port() + "/a", "https://localhost:" + canary.port() + "/a"} {
		_, _ = c.Download(context.Background(), raw, 1024, &sink)
	}
	if canary.hits.Load() != 0 {
		t.Fatalf("canary received %d download request(s)", canary.hits.Load())
	}
	if forge.hits.Load() != 0 {
		t.Fatalf("a download dialed the private base host (%d requests)", forge.hits.Load())
	}
}

// D16: hostile path parameters stay inside one escaped segment: no traversal, no query, no fragment.
func TestD16_PathParametersStayOneSegment(t *testing.T) {
	forge := newDCanary(t, nil)
	c := forge.tls()
	c.AllowPrivateBase = true
	for _, v := range []string{"x/../../admin", "a?b=1", "a#frag", "%2e%2e", "..%2f..", "a\\..\\b", "a b\r\nX-Evil: 1", "{o}", "a%00b"} {
		_, _ = c.Do(context.Background(), "GET", "/repos/{o}/{r}/git/ref/heads/x", map[string]string{"o": "owner", "r": v}, nil, nil)
	}
	for _, v := range []string{".", ".."} {
		if _, err := c.Do(context.Background(), "GET", "/repos/{o}/{r}", map[string]string{"o": "owner", "r": v}, nil, nil); err == nil {
			t.Errorf("dot segment %q accepted", v)
		}
	}
	forge.mu.Lock()
	defer forge.mu.Unlock()
	for _, uri := range forge.uris {
		if strings.Contains(uri, "?") || strings.Contains(uri, "#") || strings.Contains(uri, "/../") || strings.Contains(uri, "\n") ||
			strings.Count(strings.SplitN(uri, "/git/ref", 2)[0], "/") != 3 {
			t.Errorf("request target escaped its segment: %q", uri)
		}
	}
	if len(forge.uris) == 0 {
		t.Fatal("no request reached the fake forge (positive control)")
	}
}
