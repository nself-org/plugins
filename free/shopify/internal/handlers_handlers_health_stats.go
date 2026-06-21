package internal

import (
	"net/http"
	"time"
)

// -------------------------------------------------------------------------
// Health endpoints
// -------------------------------------------------------------------------

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "ok",
		"plugin":    "shopify",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func handleReady(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := db.Pool.Ping(r.Context()); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
				"ready":     false,
				"plugin":    "shopify",
				"error":     "Database unavailable",
				"timestamp": time.Now().UTC().Format(time.RFC3339),
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ready":     true,
			"plugin":    "shopify",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	}
}

func handleLive(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stats, _ := db.GetStats(r.Context())
		shop, _ := db.GetShop(r.Context())

		var shopInfo interface{}
		if shop != nil {
			shopInfo = map[string]interface{}{
				"name":   shop.Name,
				"domain": shop.Domain,
			}
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"alive":     true,
			"plugin":    "shopify",
			"version":   "1.0.0",
			"shop":      shopInfo,
			"stats":     stats,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	}
}

// -------------------------------------------------------------------------
// Status / Stats
// -------------------------------------------------------------------------

func handleStatus(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stats, err := db.GetStats(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		shop, _ := db.GetShop(r.Context())

		var shopInfo interface{}
		if shop != nil {
			shopInfo = map[string]interface{}{
				"name":   shop.Name,
				"domain": shop.Domain,
			}
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"shop":  shopInfo,
			"stats": stats,
		})
	}
}

func handleStats(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stats, err := db.GetStats(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, stats)
	}
}

