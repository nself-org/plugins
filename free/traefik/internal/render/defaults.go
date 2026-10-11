package render

import "strings"

// defaultServer renders default_server (EPIC D25). Its routers sit below every
// host router (priority 1 to 4), so a known Host never reaches them.
// Unknown Host on 80: the ACME webroot, /health, then a redirect or "OK".
// Unknown Host on 443: /health, then 404 with no body and no upstream (nginx
// closes the connection; the one accepted difference). TLS uses the default store.
func (b *builder) defaultServer() {
	ds := b.m.DefaultServer
	web := listener{entry: "web", tag: "web"}
	if ds.HTTP.RedirectHTTPS {
		b.addRouter("default-root", "PathPrefix(`/`)", 1, web, []string{b.redirectMW("-", "default_server.http.redirect_https", 301)}, b.responder())
	} else {
		b.addRouter("default-root", "PathPrefix(`/`)", 1, web, []string{b.respondMW(0)}, b.responder())
	}
	if ds.HTTPS.Action != "close" && ds.HTTPS.Action != "not_found" {
		b.refuse("-", "default_server.https.action", "unknown action "+ds.HTTPS.Action)
	}
	var secure []listener
	if b.hasTLS {
		secure = append(secure, listener{entry: "websecure", tag: "tls", secure: true})
		b.addRouter("default-root", "PathPrefix(`/`)", 1, secure[0], []string{b.respondMW(404)}, b.responder())
	}
	if p := ds.HTTP.HealthPath; p != nil && strings.HasPrefix(*p, "/") && !strings.Contains(*p, "`") {
		for _, l := range append([]listener{web}, secure...) {
			b.addRouter("default-health", "PathPrefix(`"+*p+"`)", 2, l, []string{b.respondMW(0)}, b.responder())
		}
	} else if p != nil {
		b.refuse("-", "default_server.http.health_path", "not an absolute path")
	}
	// The model path is only a flag: the served root is the compose mount
	// /acme-webroot (traefik-acme), whatever path routes.json names.
	if w := ds.HTTP.ACMEWebroot; w != nil {
		// Same answers as a per-route ACME location (nginx shares the snippet): GET and HEAD reach
		// the webroot at priority 4, any other method is a 403 at 3, after the body-limit check.
		// Priorities 1 (redirect or OK) and 2 (health) stay below.
		buf := b.bufferIf(b.m.Defaults.MaxBodyBytes)
		var mws []string
		if buf != "" {
			mws = append(mws, buf)
		}
		b.methodSplit("-", "default_server.http.acme_webroot", "default-acme", "PathPrefix(`"+acmePrefix+"`)", 3, web, mws, b.responder(), buf, "", []string{"GET"})
	}
}
