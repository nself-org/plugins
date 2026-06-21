package internal

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
)

func handleListInventory(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, offset := pagination(r)
		inventory, err := db.ListInventory(r.Context(), limit, offset)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		total, _ := db.CountInventory(r.Context())
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data": inventory, "total": total, "limit": limit, "offset": offset,
		})
	}
}

func handleListEvents(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		topic := r.URL.Query().Get("topic")
		limit := queryInt(r, "limit", 50)
		events, err := db.ListWebhookEvents(r.Context(), topic, limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"data": events})
	}
}

// -------------------------------------------------------------------------
// HTTP helpers
// -------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("[shopify:http] JSON encode error: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]interface{}{"error": msg})
}

func pagination(r *http.Request) (int, int) {
	limit := queryInt(r, "limit", 100)
	offset := queryInt(r, "offset", 0)
	if limit < 1 {
		limit = 100
	}
	if limit > 250 {
		limit = 250
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func queryInt(r *http.Request, key string, fallback int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}


