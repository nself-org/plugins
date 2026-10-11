package render

import "github.com/nself-org/plugins/free/traefik/internal/contract"

// Upload routes: a named route is also served on the "upload" entrypoint, a separate Traefik
// listener whose readTimeout is configurable (internal/staticcfg), so a long upload to that route
// is not cut by the 60s total deadline of web and websecure.

// uploadEntry is the name of the entrypoint in the static configuration.
const uploadEntry = "upload"

// withUpload appends the mirrored listener of a named route: a copy of its secure listener when
// it has one (same TLS options), else of its plain one. Every router the route emits for its
// listeners is then emitted on the upload entrypoint too, with the same chain, rule and priority.
// The default server has no routers there: an unknown Host on that port gets Traefik's own 404.
func (b *builder) withUpload(r contract.Route, ls []listener) []listener {
	if !b.upload[r.ID] {
		return ls
	}
	var mirror *listener
	for i := range ls {
		l := ls[i]
		l.entry = uploadEntry
		if l.secure {
			l.tag = "tls-upl"
			mirror = &l
			break
		}
		if mirror == nil {
			l.tag = "web-upl"
			mirror = &l
		}
	}
	if mirror == nil {
		return ls
	}
	return append(ls, *mirror)
}

// checkUpload refuses a named upload route id that no route has.
func (b *builder) checkUpload() {
	have := map[string]bool{}
	for _, r := range b.m.Routes {
		have[r.ID] = true
	}
	for _, id := range b.o.Upload {
		if !have[id] {
			b.refuse("-", "upload_routes", "no route "+id)
		}
	}
}
