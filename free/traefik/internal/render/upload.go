package render

import (
	"strings"

	"github.com/nself-org/plugins/free/traefik/internal/contract"
)

// Upload routes: a named route is also served on the "upload" entrypoint, a separate Traefik
// listener whose readTimeout is configurable (internal/staticcfg), so a long upload to that route
// is not cut by the 60s total deadline of web and websecure.

// uploadEntry is the name of the entrypoint in the static configuration.
const uploadEntry = "upload"

// withUpload appends the mirrored listener of a named route: a copy of its secure listener when
// it has one (same TLS options), else of its plain one. Every router the route emits for its
// listeners is then emitted on the upload entrypoint too, with the same chain, rule and priority.
// The default server has no routers there: an unknown Host on that port gets Traefik's own 404.
// A host owned by an unnamed route has no router there either, so a named wildcard that covers it
// would answer it with the wildcard's own upstream: checkUpload refuses that overlap.
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

// wildSuffix is the suffix (".example.test") a server name covers, or "" for an exact name.
func wildSuffix(n string) string {
	n = strings.ToLower(n)
	if strings.HasPrefix(n, "*.") {
		return n[1:]
	}
	if strings.HasPrefix(n, ".") {
		return n
	}
	return ""
}

// checkUploadOverlap refuses a named route whose wildcard covers a server name that an unnamed
// route owns more specifically (an exact name, or a wildcard with a longer suffix). On the web
// listeners the unnamed route wins that host; on the upload listener it has no router, so the
// wildcard would answer it. Name the overlapping route too, or drop the wildcard.
func (b *builder) checkUploadOverlap() {
	for _, r := range b.m.Routes {
		if !b.upload[r.ID] {
			continue
		}
		for _, wn := range r.ServerNames {
			s := wildSuffix(wn)
			if s == "" {
				continue
			}
			for _, o := range b.m.Routes {
				if b.upload[o.ID] {
					continue
				}
				for _, on := range o.ServerNames {
					lo := strings.ToLower(on)
					os := wildSuffix(on)
					covered := os == "" && strings.HasSuffix(lo, s) // exact name under the suffix
					if os != "" {
						covered = len(os) > len(s) && strings.HasSuffix(os, s)
					}
					if covered {
						b.refuse(r.ID, "upload_routes", "unnamed route "+o.ID+" owns "+on+", which this wildcard would answer on the upload port")
					}
				}
			}
		}
	}
}
