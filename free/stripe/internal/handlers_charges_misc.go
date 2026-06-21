package internal

import (
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"net/http"
	"os"
	"strconv"
	"strings"
)

func (s *Server) handleListPayouts(w http.ResponseWriter, r *http.Request) {
	db := s.scopedDB(r)
	limit, offset := parsePagination(r)
	data, err := db.ListPayouts(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	total, _ := db.CountPayouts(r.Context())
	writeJSON(w, http.StatusOK, ListResponse{Data: data, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleGetPayout(w http.ResponseWriter, r *http.Request) {
	s.handleGetByID(w, r, "np_stripe_payouts")
}

func (s *Server) handleListDisputes(w http.ResponseWriter, r *http.Request) {
	db := s.scopedDB(r)
	limit, offset := parsePagination(r)
	data, err := db.ListDisputes(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	total, _ := db.CountDisputes(r.Context())
	writeJSON(w, http.StatusOK, ListResponse{Data: data, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleGetDispute(w http.ResponseWriter, r *http.Request) {
	s.handleGetByID(w, r, "np_stripe_disputes")
}

func (s *Server) handleListEvents(w http.ResponseWriter, r *http.Request) {
	db := s.scopedDB(r)
	limit, offset := parsePagination(r)
	eventType := r.URL.Query().Get("type")
	data, err := db.ListEvents(r.Context(), eventType, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":   data,
		"limit":  limit,
		"offset": offset,
	})
}

func (s *Server) handleListCheckoutSessions(w http.ResponseWriter, r *http.Request) {
	db := s.scopedDB(r)
	limit, offset := parsePagination(r)
	data, err := db.ListCheckoutSessions(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	total, _ := db.CountCheckoutSessions(r.Context())
	writeJSON(w, http.StatusOK, ListResponse{Data: data, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleGetCheckoutSession(w http.ResponseWriter, r *http.Request) {
	s.handleGetByID(w, r, "np_stripe_checkout_sessions")
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	db := s.scopedDB(r)
	stats, err := db.GetStats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// ============================================================================
// Helpers
// ============================================================================

func (s *Server) handleGetByID(w http.ResponseWriter, r *http.Request, table string) {
	db := s.scopedDB(r)
	id := chi.URLParam(r, "id")
	data, err := db.getRow(r.Context(), table, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if data == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Not found"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

func parsePagination(r *http.Request) (int, int) {
	limit := 100
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if l, err := strconv.Atoi(v); err == nil && l > 0 && l <= 1000 {
			limit = l
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if o, err := strconv.Atoi(v); err == nil && o >= 0 {
			offset = o
		}
	}
	return limit, offset
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

// stripeAllowedOrigins returns the per-request origin-allowlist populated
// from STRIPE_ALLOWED_ORIGINS (csv). An empty env var means no cross-origin
// requests are allowed. Wildcard ("*") is never accepted since this plugin
// emits Access-Control-Allow-Credentials: true.
func stripeAllowedOrigins() map[string]bool {
	set := make(map[string]bool)
	for _, o := range strings.Split(os.Getenv("STRIPE_ALLOWED_ORIGINS"), ",") {
		o = strings.TrimSpace(strings.ToLower(o))
		if o == "" || o == "*" {
			continue
		}
		set[o] = true
	}
	return set
}

// corsMiddleware enforces explicit per-origin CORS. If the request Origin
// is not in the allowlist, CORS headers are NOT emitted and the browser
// blocks the response. Credentials are only permitted alongside an echoed,
// allowlisted origin — never with a wildcard.
//
// Webhook endpoints (invoked server-to-server by Stripe) must NOT be
// mounted behind this middleware; Stripe calls them directly and no
// Origin header is involved. See NewRouter for route grouping.
func corsMiddleware(next http.Handler) http.Handler {
	allowed := stripeAllowedOrigins()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Vary", "Origin")
		origin := r.Header.Get("Origin")
		originAllowed := origin != "" && allowed[strings.ToLower(origin)]
		if originAllowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Source-Account-ID, Stripe-Signature")
			w.Header().Set("Access-Control-Max-Age", "600")
		}

		if r.Method == "OPTIONS" {
			if !originAllowed {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
