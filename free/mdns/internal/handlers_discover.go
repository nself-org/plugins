package internal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	sdk "github.com/nself-org/plugin-sdk"
)

// Size-cap exception: single-responsibility HTTP route handler — 61L of request decode + validate + DB op + response encode; splitting adds indirection without cohesion gain.
func handleDiscover(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req DiscoverRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}

		// If specific services were provided (from CLI host-side mDNS scan),
		// upsert them into the discovery log.
		if len(req.Services) > 0 {
			var results []DiscoveryLogRecord
			for _, entry := range req.Services {
				if entry.ServiceName == "" || entry.Host == "" {
					continue
				}
				if entry.ServiceType == "" && req.ServiceType != "" {
					entry.ServiceType = req.ServiceType
				}
				if entry.ServiceType == "" {
					entry.ServiceType = "_ntv._tcp"
				}

				rec, err := db.UpsertDiscovery(entry)
				if err != nil {
					sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to upsert discovery: %w", err))
					return
				}
				results = append(results, *rec)
			}
			if results == nil {
				results = []DiscoveryLogRecord{}
			}
			sdk.Respond(w, http.StatusOK, map[string]interface{}{
				"discovered": results,
				"count":      len(results),
			})
			return
		}

		// No services provided: return existing discovery log entries
		// (the actual mDNS multicast scan is done by the host CLI).
		serviceType := req.ServiceType
		if serviceType == "" {
			serviceType = "_ntv._tcp"
		}
		available := true
		records, total, err := db.ListDiscoveries(serviceType, &available, 100, 0)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to list discoveries: %w", err))
			return
		}
		if records == nil {
			records = []DiscoveryLogRecord{}
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"discovered": records,
			"count":      total,
		})
	}
}

func handleListDiscovered(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		serviceType := r.URL.Query().Get("service_type")

		var isAvailable *bool
		if v := r.URL.Query().Get("is_available"); v != "" {
			b, err := strconv.ParseBool(v)
			if err == nil {
				isAvailable = &b
			}
		}

		limit := 50
		if v := r.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
				limit = n
			}
		}

		offset := 0
		if v := r.URL.Query().Get("offset"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				offset = n
			}
		}

		records, total, err := db.ListDiscoveries(serviceType, isAvailable, limit, offset)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to list discoveries: %w", err))
			return
		}
		if records == nil {
			records = []DiscoveryLogRecord{}
		}
		sdk.Respond(w, http.StatusOK, ListResponse{
			Items:  records,
			Total:  total,
			Limit:  limit,
			Offset: offset,
		})
	}
}

// --- Stats handler ---

func handleStats(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stats, err := db.GetStats()
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to get stats: %w", err))
			return
		}
		sdk.Respond(w, http.StatusOK, stats)
	}
}
