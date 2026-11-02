package render

import (
	"github.com/nself-org/plugins/free/traefik/internal/contract"
	"strings"
	"testing"
)

// D-0769: compressible proxied routes must carry Vary: Accept-Encoding so caches
// distinguish compressed and uncompressed responses, matching nginx gzip_vary.
func TestCompressibleRouteAddsVaryAcceptEncoding(t *testing.T) {
	m := fixture(t, "http-full")
	if !m.Defaults.Gzip.Enabled {
		t.Fatal("test premise: http-full must have gzip enabled")
	}
	out, err := Render(m, Options{})
	if err != nil {
		t.Fatal(err)
	}
	doc := parse(t, out)
	mws := doc["http"]["middlewares"]
	var varyName string
	for name, mw := range mws {
		if h, ok := mw.(map[string]any)["headers"].(map[string]any); ok {
			if crh, ok := h["customResponseHeaders"].(map[string]any); ok {
				if v, ok := crh["Vary"].(string); ok && strings.Contains(v, "Accept-Encoding") {
					varyName = name
					break
				}
			}
		}
	}
	if varyName == "" {
		t.Fatal("no headers middleware with Vary: Accept-Encoding rendered")
	}
	seen := 0
	for _, r := range doc["http"]["routers"] {
		spec := r.(map[string]any)
		chain, _ := spec["middlewares"].([]any)
		hasCompress, hasVary := false, false
		for _, n := range chain {
			ns := n.(string)
			if strings.HasPrefix(ns, "compress-") {
				hasCompress = true
			}
			if ns == varyName {
				hasVary = true
			}
		}
		if hasCompress && !hasVary {
			t.Errorf("router %s has compress but no Vary middleware", spec["rule"])
		}
		if hasCompress && hasVary {
			seen++
		}
	}
	if seen == 0 {
		t.Error("no compressible router carried the Vary middleware; test checked nothing")
	}
}

// D-0769: websocket (upgraded) routes on HTTPS must add Strict-Transport-Security
// because browsers ignore CSP/X-Frame-Options on a 101 handshake; HSTS is the one
// security header that still protects the upgrade.
func TestWebSocketHTTPSRouteAddsHSTS(t *testing.T) {
	l := loc(t, "proxy", "/ws", "prefix")
	l.WebSocket = true
	r := mkRoute(t, "hand:ws#1", []string{"ws.example.test"}, loc(t, "proxy", "/", "prefix"), l)
	r.Listen.HTTPS = true
	r.TLS = &contract.RouteTLS{SSLDir: "example-org"}
	m := fixture(t, "plugin-modelled")
	m.Routes = []contract.Route{r}
	root := sslTree(t, m)
	out, err := Render(m, Options{SSLRoot: root, CertRoot: "/nself/ssl"})
	if err != nil {
		t.Fatal(err)
	}
	http := httpSection(t, out)
	mws := http["middlewares"]
	var hstsName string
	for name, mw := range mws {
		if h, ok := mw.(map[string]any)["headers"].(map[string]any); ok {
			if crh, ok := h["customResponseHeaders"].(map[string]any); ok {
				if _, ok := crh["Strict-Transport-Security"]; ok {
					hstsName = name
					break
				}
			}
		}
	}
	if hstsName == "" {
		t.Fatal("no headers middleware with Strict-Transport-Security rendered for websocket route")
	}
	seen := 0
	for name, r := range http["routers"] {
		spec := r.(map[string]any)
		if !strings.Contains(name, "-tls") {
			continue
		}
		chain, _ := spec["middlewares"].([]any)
		has := false
		for _, n := range chain {
			if n.(string) == hstsName {
				has = true
				break
			}
		}
		if strings.Contains(spec["rule"].(string), "/ws") && !has {
			t.Errorf("websocket TLS router %s lacks HSTS middleware", name)
		}
		if has {
			seen++
		}
	}
	if seen == 0 {
		t.Error("no TLS websocket router carried the HSTS middleware; test checked nothing")
	}
}

// D-0769: HSTS must not be added to plain HTTP routers (browsers would cache it
// before the redirect, and nginx does not set it there either).
func TestWebSocketHTTPRouteDoesNotAddHSTS(t *testing.T) {
	l := loc(t, "proxy", "/ws", "prefix")
	l.WebSocket = true
	r := mkRoute(t, "hand:wsplain#1", []string{"wsplain.example.test"}, loc(t, "proxy", "/", "prefix"), l)
	r.Listen.HTTP, r.Listen.HTTPS = true, false
	r.TLS = nil
	out := mustRender(t, r)
	http := httpSection(t, out)
	for name, r := range http["routers"] {
		spec := r.(map[string]any)
		if !strings.Contains(name, "-web") || strings.Contains(name, "-tls") {
			continue
		}
		chain, _ := spec["middlewares"].([]any)
		for _, n := range chain {
			mw := http["middlewares"][n.(string)].(map[string]any)
			if h, ok := mw["headers"].(map[string]any); ok {
				if crh, ok := h["customResponseHeaders"].(map[string]any); ok {
					if _, ok := crh["Strict-Transport-Security"]; ok {
						t.Errorf("HTTP router %s must not carry HSTS", name)
					}
				}
			}
		}
	}
}
