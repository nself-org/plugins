package main

import (
	"fmt"
	"strings"
)

// hostDot: nginx strips the trailing dot of an absolute Host ("w.example.test."), so a wildcard or
// leading-dot server name still serves it. The renderer's HostRegexp accepts `\.?$` for the same
// result; both proxies must answer the same status.
func hostDot(r *runner, m routes, need bool) {
	n := 0
	for _, rt := range m.Routes {
		name := rt.ServerNames[0]
		if (!strings.HasPrefix(name, "*.") && !strings.HasPrefix(name, ".")) || !hasRoot(rt) || (!rt.Listen.HTTP && !rt.Listen.HTTPS) {
			continue
		}
		q := req{scheme: scheme(rt), host: hostOf(rt) + ".", method: "GET", path: "/"}
		a, b := r.n.do(q), r.t.do(q)
		switch {
		case a.err != nil || b.err != nil:
			r.fail(rt.ID+" trailing-dot Host", fmt.Sprintf("transport error nginx=%v traefik=%v", a.err, b.err))
		case a.status == 429 || b.status == 429: // earlier traffic drained this bucket: nothing to compare
		case a.status != b.status:
			r.fail(rt.ID+" trailing-dot Host", fmt.Sprintf("status nginx=%d traefik=%d", a.status, b.status))
		default:
			n++
			r.ok(fmt.Sprintf("%s trailing-dot Host %s -> %d", rt.ID, q.host, a.status))
		}
	}
	if need && n == 0 {
		r.fail("trailing-dot Host", "no wildcard route compared")
	}
}
