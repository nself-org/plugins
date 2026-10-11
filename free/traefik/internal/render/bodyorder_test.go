package render

import (
	"strings"
	"testing"
)

// On a proxied "/" location the body limit (buffering) comes AFTER the rate and connection
// limits. Buffering reads the whole in-limit body into memory, and rate and conn are what bound
// concurrent buffering; moving it first would let a limited client make Traefik buffer without
// bound. The price is a 429 where nginx gives 413 on an exhausted zone (documented in the wiki).
// Do not "fix" this order.
func TestProxiedBodyLimitFollowsRateAndConnLimits(t *testing.T) {
	m := fixture(t, "tls-full")
	var id string
	for ri := range m.Routes {
		r := &m.Routes[ri]
		if r.ID != "core:auth" || r.RateLimit == nil || r.RateLimit.ConnLimit <= 0 {
			continue
		}
		size := int64(1048576)
		for li := range r.Locations {
			if l := &r.Locations[li]; l.Path == "/" && l.Match == "prefix" && l.Upstream != nil {
				l.MaxBodyBytes = &size
				id = r.ID
			}
		}
	}
	if id == "" {
		t.Fatal("fixture has no route with a rate limit, conn_limit and a proxied / prefix: the test checked nothing")
	}
	out, err := Render(m, Options{SSLRoot: sslTree(t, m), CertRoot: "/nself/ssl"})
	if err != nil {
		t.Fatal(err)
	}
	mws := httpSection(t, out)["middlewares"]
	checked := 0
	for name, v := range routersOf(t, out) {
		if !strings.HasPrefix(name, rname(id)+"-") {
			continue
		}
		var rate, conn, buf = -1, -1, -1
		chain, _ := v["middlewares"].([]any)
		for i, c := range chain {
			def, _ := mws[c.(string)].(map[string]any)
			switch {
			case def["rateLimit"] != nil:
				rate = i
			case def["inFlightReq"] != nil:
				conn = i
			case def["buffering"] != nil:
				buf = i
			}
		}
		if conn < 0 || buf < 0 {
			continue // not the "/" router
		}
		checked++
		if rate < 0 || rate > buf || conn > buf {
			t.Errorf("%s: want rateLimit and inFlightReq before buffering, got rate=%d conn=%d buffering=%d in %v", name, rate, conn, buf, chain)
		}
	}
	if checked == 0 {
		t.Fatal("no router of the route carried inFlightReq and buffering: the test checked nothing")
	}
}
