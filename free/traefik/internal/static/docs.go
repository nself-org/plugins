package static

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// serveDocs serves only the two core documentation files, without directory
// listings or symlink traversal. The renderer supplies the location headers.
func serveDocs(w http.ResponseWriter, r *http.Request, dist string) {
	name := strings.TrimPrefix(r.URL.Path, "/__nself/docs/")
	if name != "openapi.json" && name != "scalar.html" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	root, err := os.OpenRoot(dist)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	defer func() { _ = root.Close() }()
	fi, err := root.Lstat(name)
	if err != nil || !fi.Mode().IsRegular() {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	f, err := root.Open(name)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	defer func() { _ = f.Close() }()
	fi, err = f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	typ := "application/json"
	if filepath.Ext(name) == ".html" {
		typ = "text/html"
	}
	w.Header().Set("Content-Type", typ)
	w.Header().Set("ETag", fmt.Sprintf("\"%x-%x\"", fi.ModTime().Unix(), fi.Size()))
	http.ServeContent(w, r, name, fi.ModTime(), f)
}
