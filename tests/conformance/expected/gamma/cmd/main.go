// Command gamma is a fixture plugin: a dependency-free Go service.
package main

import (
	"log"
	"net/http"
)

func main() {
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	log.Fatal(http.ListenAndServe(":3903", nil))
}
