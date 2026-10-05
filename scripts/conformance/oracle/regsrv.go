package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
)

func main() {
	dir := os.Args[1]
	fs := http.FileServer(http.Dir(dir))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/registry.json" {
			w.Header().Set("Content-Type", "application/json")
			http.ServeFile(w, r, filepath.Join(dir, "registry.json"))
			return
		}
		fs.ServeHTTP(w, r)
	})
	log.Fatal(http.ListenAndServe("127.0.0.1:8099", nil))
}
