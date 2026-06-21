package internal

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	sdk "github.com/nself-org/plugin-sdk"
)

// =========================================================================
// Movies
// =========================================================================

func handleCreateMovie(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateMovieRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.Title == "" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("title is required"))
			return
		}

		qp := "balanced"
		if req.QualityProfile != nil {
			qp = *req.QualityProfile
		}
		autoDownload := true
		if req.AutoDownload != nil {
			autoDownload = *req.AutoDownload
		}
		autoUpgrade := false
		if req.AutoUpgrade != nil {
			autoUpgrade = *req.AutoUpgrade
		}

		accountID := sourceAccountID(r)
		movie, err := db.CreateMovieMonitoring(accountID, req.Title, req.TmdbID, qp, autoDownload, autoUpgrade)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to create movie: %w", err))
			return
		}
		sdk.Respond(w, http.StatusCreated, map[string]interface{}{"movie": movie})
	}
}

func handleListMovies(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID := sourceAccountID(r)
		movies, err := db.ListMovieMonitoring(accountID)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to list movies: %w", err))
			return
		}
		if movies == nil {
			movies = []MovieMonitoring{}
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"movies": movies})
	}
}

func handleUpdateMovie(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var req UpdateMovieRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}

		movie, err := db.UpdateMovieMonitoring(id, req)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to update movie: %w", err))
			return
		}
		if movie == nil {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("Movie not found"))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"movie": movie})
	}
}

func handleDeleteMovie(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		deleted, err := db.DeleteMovieMonitoring(id)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to delete movie: %w", err))
			return
		}
		if !deleted {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("Movie not found"))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"deleted": true})
	}
}

