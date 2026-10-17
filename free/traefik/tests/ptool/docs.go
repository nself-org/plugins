package main

import "fmt"

func must(err error) {
	if err != nil {
		die(err)
	}
}

// docsPair checks successful file bodies and configured headers explicitly.
// nginx adds its default MIME type before the configured Content-Type. Check
// that final explicit type; Traefik replaces the default through middleware.
func docsPair(r *runner, rt route, path string, headers map[string]string) {
	for _, method := range []string{"GET", "HEAD"} {
		q := req{scheme: scheme(rt), host: hostOf(rt), method: method, path: path + "?schema=1"}
		a, b := r.pairMode(rt.ID+" "+method+" "+path, "body", plain(q), 200)
		for k, want := range headers {
			for _, res := range []result{a, b} {
				values := res.hdr.Values(k)
				if k == "Content-Type" && len(values) > 1 {
					values = values[len(values)-1:]
				}
				if len(values) == 0 {
					r.fail(path+" "+k, "missing response header")
				}
				for _, got := range values {
					if got != want {
						r.fail(path+" "+k, fmt.Sprintf("got %q, want %q", got, want))
					}
				}
			}
		}
		if method == "GET" && (a.size == 0 || b.size == 0) {
			r.fail(path, "documentation file is empty")
		}
	}
	for _, p := range []string{path + "/x", path + "x"} {
		q := req{scheme: scheme(rt), host: hostOf(rt), method: "GET", path: p}
		r.pairMode(rt.ID+" alias suffix "+p, "status", plain(q), 404)
	}
	q := req{scheme: scheme(rt), host: hostOf(rt), method: "POST", path: path}
	r.pairMode(rt.ID+" POST "+path, "status", plain(q), 405)
}
