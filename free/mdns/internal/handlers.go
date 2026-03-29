package internal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	sdk "github.com/nself-org/plugin-sdk"
)

var startTime = time.Now()

// RegisterRoutes mounts all mDNS API routes on the given router.
func RegisterRoutes(r chi.Router, db *DB) {
	// Operational endpoints
	r.Get("/ready", handleReady(db))
	r.Get("/live", handleLive(db))

	// Service CRUD
	r.Route("/api", func(r chi.Router) {
		r.Post("/services", handleCreateService(db))
		r.Get("/services", handleListServices(db))
		r.Get("/services/{id}", handleGetService(db))
		r.Put("/services/{id}", handleUpdateService(db))
		r.Delete("/services/{id}", handleDeleteService(db))
		r.Post("/services/{id}/advertise", handleAdvertise(db))
		r.Post("/services/{id}/stop", handleStopAdvertise(db))

		// Discovery
		r.Post("/discover", handleDiscover(db))
		r.Get("/discovered", handleListDiscovered(db))

		// Stats
		r.Get("/stats", handleStats(db))
	})
}

// --- Operational handlers ---

func handleReady(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := db.Ping(); err != nil {
			sdk.Error(w, http.StatusServiceUnavailable, fmt.Errorf("database not ready: %w", err))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]string{"status": "ready"})
	}
}

func handleLive(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)

		stats, _ := db.GetStats()

		resp := map[string]interface{}{
			"status": "live",
			"uptime": time.Since(startTime).String(),
			"memory": map[string]interface{}{
				"alloc_mb":       fmt.Sprintf("%.2f", float64(mem.Alloc)/1024/1024),
				"total_alloc_mb": fmt.Sprintf("%.2f", float64(mem.TotalAlloc)/1024/1024),
				"sys_mb":         fmt.Sprintf("%.2f", float64(mem.Sys)/1024/1024),
				"num_gc":         mem.NumGC,
			},
			"goroutines": runtime.NumGoroutine(),
		}

		if stats != nil {
			resp["stats"] = stats
		}

		sdk.Respond(w, http.StatusOK, resp)
	}
}

// --- Service CRUD handlers ---

func handleCreateService(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateServiceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.ServiceName == "" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("service_name is required"))
			return
		}
		if req.Port <= 0 {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("port must be a positive integer"))
			return
		}

		svc, err := db.CreateService(req)
		if err != nil {
			sdk.Error(w, http.StatusConflict, fmt.Errorf("failed to create service: %w", err))
			return
		}
		sdk.Respond(w, http.StatusCreated, svc)
	}
}

func handleListServices(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		serviceType := r.URL.Query().Get("service_type")

		var isAdvertised *bool
		if v := r.URL.Query().Get("is_advertised"); v != "" {
			b, err := strconv.ParseBool(v)
			if err == nil {
				isAdvertised = &b
			}
		}

		var isActive *bool
		if v := r.URL.Query().Get("is_active"); v != "" {
			b, err := strconv.ParseBool(v)
			if err == nil {
				isActive = &b
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

		services, total, err := db.ListServices(serviceType, isAdvertised, isActive, limit, offset)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to list services: %w", err))
			return
		}
		if services == nil {
			services = []ServiceRecord{}
		}
		sdk.Respond(w, http.StatusOK, ListResponse{
			Items:  services,
			Total:  total,
			Limit:  limit,
			Offset: offset,
		})
	}
}

func handleGetService(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		svc, err := db.GetService(id)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to get service: %w", err))
			return
		}
		if svc == nil {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("service not found"))
			return
		}
		sdk.Respond(w, http.StatusOK, svc)
	}
}

func handleUpdateService(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var req UpdateServiceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}

		svc, err := db.UpdateService(id, req)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to update service: %w", err))
			return
		}
		if svc == nil {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("service not found"))
			return
		}
		sdk.Respond(w, http.StatusOK, svc)
	}
}

func handleDeleteService(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		deleted, err := db.DeleteService(id)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to delete service: %w", err))
			return
		}
		if !deleted {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("service not found"))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"success": true,
		})
	}
}

func handleAdvertise(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		svc, err := db.SetAdvertised(id, true)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to start advertising: %w", err))
			return
		}
		if svc == nil {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("service not found"))
			return
		}
		sdk.Respond(w, http.StatusOK, svc)
	}
}

func handleStopAdvertise(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		svc, err := db.SetAdvertised(id, false)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to stop advertising: %w", err))
			return
		}
		if svc == nil {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("service not found"))
			return
		}
		sdk.Respond(w, http.StatusOK, svc)
	}
}

// --- Discovery handlers ---

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
