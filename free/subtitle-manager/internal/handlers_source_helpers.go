package internal

import (
	"net/http"
	"strconv"
)

func getSourceAccountID(r *http.Request) string {
	id := r.Header.Get("X-Source-Account-ID")
	if id == "" {
		return "primary"
	}
	return id
}

func parseIntParam(s string, defaultVal int) int {
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return v
}
