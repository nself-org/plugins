// Package internal implements the GDPR plugin HTTP handlers. The plugin
// exposes a small REST API used by the Admin UI and web/cloud to trigger
// and monitor GDPR export/delete requests without requiring direct
// database access from the frontend.
package internal

import (
	"encoding/json"
	"net/http"
	"strings"
)

// Handler returns an http.Handler that serves the GDPR plugin REST API.
// secret is the PLUGIN_INTERNAL_SECRET used for authentication.
func Handler(secret string, svc *Service) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /health", http.HandlerFunc(handleHealth))
	mux.Handle("POST /gdpr/export", authMiddleware(secret, http.HandlerFunc(svc.HandleExport)))
	mux.Handle("POST /gdpr/delete", authMiddleware(secret, http.HandlerFunc(svc.HandleDelete)))
	mux.Handle("GET /gdpr/request/{id}", authMiddleware(secret, http.HandlerFunc(svc.HandleGetRequest)))
	mux.Handle("GET /gdpr/requests", authMiddleware(secret, http.HandlerFunc(svc.HandleListRequests)))
	mux.Handle("POST /gdpr/registry", authMiddleware(secret, http.HandlerFunc(svc.HandleRegisterPlugin)))

	return mux
}

// handleHealth returns 200 OK for liveness probes.
func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// authMiddleware validates the PLUGIN_INTERNAL_SECRET bearer token.
func authMiddleware(secret string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		token := strings.TrimPrefix(auth, "Bearer ")
		if secret != "" && token != secret {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// writeJSON serialises v to JSON and writes it to w.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// decodeJSON reads and validates the request body as JSON into dst.
func decodeJSON(r *http.Request, dst interface{}) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}
