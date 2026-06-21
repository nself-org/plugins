package internal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"github.com/go-chi/chi/v5"
	sdk "github.com/nself-org/plugin-sdk"
)

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
