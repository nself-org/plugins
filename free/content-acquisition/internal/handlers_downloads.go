package internal

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	sdk "github.com/nself-org/plugin-sdk"
)

// =========================================================================
// Downloads
// =========================================================================

func handleCreateDownload(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateDownloadRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.ContentType == "" || req.Title == "" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("contentType and title are required"))
			return
		}

		qp := "balanced"
		if req.QualityProfile != nil {
			qp = *req.QualityProfile
		}

		accountID := sourceAccountID(r)
		dl, err := db.CreateDownload(accountID, req.ContentType, req.Title, req.MagnetURI, qp, req.ShowID, req.SeasonNumber, req.EpisodeNumber, req.TmdbID)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to create download: %w", err))
			return
		}

		// Add to download queue
		if err := db.AddToDownloadQueue(dl.ID); err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to add to download queue: %w", err))
			return
		}

		sdk.Respond(w, http.StatusCreated, map[string]interface{}{"download": dl})
	}
}

func handleListDownloads(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID := sourceAccountID(r)
		var stateFilter *string
		if v := r.URL.Query().Get("status"); v != "" {
			stateFilter = &v
		}
		downloads, err := db.ListDownloads(accountID, stateFilter)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to list downloads: %w", err))
			return
		}
		if downloads == nil {
			downloads = []Download{}
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"downloads": downloads})
	}
}

func handleGetDownload(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		dl, err := db.GetDownload(id)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to get download: %w", err))
			return
		}
		if dl == nil {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("Download not found"))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"download": dl})
	}
}

func handleDeleteDownload(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		dl, err := db.GetDownload(id)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to get download: %w", err))
			return
		}
		if dl == nil {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("Download not found"))
			return
		}

		// Transition to cancelled if not already in a terminal state
		terminalStates := map[string]bool{"completed": true, "failed": true, "cancelled": true}
		if !terminalStates[dl.State] {
			meta, _ := json.Marshal(map[string]string{"reason": "user_cancelled"})
			_ = db.UpdateDownloadState(id, "cancelled", meta)
		}

		_ = db.RemoveFromDownloadQueue(id)
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"cancelled": true, "download_id": id})
	}
}

func handlePauseDownload(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		dl, err := db.GetDownload(id)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to get download: %w", err))
			return
		}
		if dl == nil {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("Download not found"))
			return
		}

		meta, _ := json.Marshal(map[string]string{"reason": "user_paused"})
		if err := db.UpdateDownloadState(id, "paused", meta); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("%v", err))
			return
		}

		updated, _ := db.GetDownload(id)
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"download": updated})
	}
}

func handleResumeDownload(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		dl, err := db.GetDownload(id)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to get download: %w", err))
			return
		}
		if dl == nil {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("Download not found"))
			return
		}
		if dl.State != "paused" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("Download is not paused"))
			return
		}

		// Find the state before pause from history
		history, err := db.GetDownloadStateHistory(id)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to get history: %w", err))
			return
		}

		resumeState := "downloading"
		for i := len(history) - 1; i >= 0; i-- {
			if history[i].ToState == "paused" && history[i].FromState != nil {
				resumeState = *history[i].FromState
				break
			}
		}

		meta, _ := json.Marshal(map[string]string{"reason": "user_resumed"})
		if err := db.UpdateDownloadState(id, resumeState, meta); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("%v", err))
			return
		}

		updated, _ := db.GetDownload(id)
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"download": updated})
	}
}

func handleRetryDownload(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		dl, err := db.GetDownload(id)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to get download: %w", err))
			return
		}
		if dl == nil {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("Download not found"))
			return
		}
		if dl.State != "failed" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("Only failed downloads can be retried"))
			return
		}

		meta, _ := json.Marshal(map[string]string{"reason": "user_retry"})
		if err := db.UpdateDownloadState(id, "created", meta); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("%v", err))
			return
		}

		newRetryCount := dl.RetryCount + 1
		_ = db.UpdateDownloadFields(id, &newRetryCount, nil)
		_ = db.AddToDownloadQueue(id)

		updated, _ := db.GetDownload(id)
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"download": updated})
	}
}

func handleGetDownloadHistory(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		dl, err := db.GetDownload(id)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to get download: %w", err))
			return
		}
		if dl == nil {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("Download not found"))
			return
		}

		history, err := db.GetDownloadStateHistory(id)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to get history: %w", err))
			return
		}
		if history == nil {
			history = []DownloadStateTransition{}
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"download_id": id, "history": history})
	}
}

