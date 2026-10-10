// Package static is the traefik-acme helper: it serves the ACME HTTP-01 webroot
// (nginx parity: one bare token under /.well-known/acme-challenge/, no
// subdirectories, no traversal, no symlinks, GET and HEAD only) and answers the
// fixed statuses the rendered configuration routes to it (/__nself/ok and
// /__nself/status/<code>). It never proxies and never lists a directory.
//
// Inputs: a webroot directory. Outputs: an http.Handler. Constraints: stdlib only.
package static

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var tokenRE = regexp.MustCompile(`^/[.]well-known/acme-challenge/[A-Za-z0-9_-]+$`)

// Handler serves root as the ACME webroot plus the fixed responder paths.
func Handler(root string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case p == "/__nself/ok":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("OK"))
		case strings.HasPrefix(p, "/__nself/status/"):
			code, err := strconv.Atoi(strings.TrimPrefix(p, "/__nself/status/"))
			if err != nil || code < 200 || code > 599 || code == 204 || code == 205 || code == 304 {
				http.NotFound(w, r)
				return
			}
			w.WriteHeader(code)
		default:
			serveToken(w, r, root)
		}
	})
}

// serveToken mirrors the nginx challenge location: the URI check answers 404
// before the method check answers 403, as in nginx's rewrite-then-access order.
func serveToken(w http.ResponseWriter, r *http.Request, root string) {
	if !tokenRE.MatchString(r.URL.Path) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	file := filepath.Join(root, filepath.FromSlash(r.URL.Path))
	rel, err := filepath.Rel(root, file)
	if err != nil || strings.HasPrefix(rel, "..") {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	cur := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		cur = filepath.Join(cur, part)
		fi, err := os.Lstat(cur)
		if err != nil || fi.Mode()&os.ModeSymlink != 0 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
	}
	f, err := os.Open(file)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("ETag", fmt.Sprintf("\"%x-%x\"", fi.ModTime().Unix(), fi.Size()))
	http.ServeContent(w, r, "", fi.ModTime(), f)
}
