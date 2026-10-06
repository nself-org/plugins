package main

import (
	"net/http"
	"time"
)

var httptimeout = struct {
	Default *http.Client
}{
	Default: &http.Client{Timeout: 30 * time.Second},
}
