package internal

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Handlers holds shared dependencies for all HTTP handlers.
type Handlers struct {
	DB  *pgxpool.Pool
	Key [32]byte // AES-256 encryption key for credentials
}

// sa returns the source account ID from the X-Source-Account-ID header,
// falling back to "primary" if not present.
func (h *Handlers) sa(r *http.Request) string {
	if v := r.Header.Get("X-Source-Account-ID"); v != "" {
		return v
	}
	return "primary"
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

// =============================================================================
// Accounts
// =============================================================================

// HandleCreateAccount handles POST /post/accounts.
func (h *Handlers) HandleCreateAccount(w http.ResponseWriter, r *http.Request) {
	var req CreateAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	sourceID := h.sa(r)
	if req.SourceAccountID != nil && *req.SourceAccountID != "" {
		sourceID = *req.SourceAccountID
	}

	credsJSON, err := json.Marshal(req.Credentials)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "marshal credentials"})
		return
	}
	encrypted, err := h.encryptCredentials(credsJSON)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	encMap := map[string]string{"__enc": encrypted}
	encBytes, _ := json.Marshal(encMap)

	var id string
	var createdAt time.Time
	err = h.DB.QueryRow(r.Context(),
		`INSERT INTO np_post_accounts (source_account_id, platform, name, endpoint, credentials)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, created_at`,
		sourceID, req.Platform, req.Name, req.Endpoint, string(encBytes),
	).Scan(&id, &createdAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, PostAccountView{
		ID:              id,
		SourceAccountID: sourceID,
		Platform:        req.Platform,
		Name:            req.Name,
		Endpoint:        req.Endpoint,
		CreatedAt:       createdAt,
	})
}

// HandleListAccounts handles GET /post/accounts.
func (h *Handlers) HandleListAccounts(w http.ResponseWriter, r *http.Request) {
	sourceID := h.sa(r)
	rows, err := h.DB.Query(r.Context(),
		`SELECT id, source_account_id, platform, name, endpoint, created_at
		 FROM np_post_accounts WHERE source_account_id = $1 ORDER BY created_at DESC`,
		sourceID,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	accounts := make([]PostAccountView, 0)
	for rows.Next() {
		var a PostAccountView
		if err := rows.Scan(&a.ID, &a.SourceAccountID, &a.Platform, &a.Name, &a.Endpoint, &a.CreatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		accounts = append(accounts, a)
	}
	writeJSON(w, http.StatusOK, accounts)
}

// HandleDeleteAccount handles DELETE /post/accounts/{id}.
func (h *Handlers) HandleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	// Multi-app isolation: scope the delete to the caller's source account so one
	// account cannot delete another account's post account by guessing its id.
	sourceID := h.sa(r)
	ct, err := h.DB.Exec(r.Context(), `DELETE FROM np_post_accounts WHERE id = $1 AND source_account_id = $2`, id, sourceID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if ct.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "account not found"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// =============================================================================
// Publish
// =============================================================================

// HandlePublish handles POST /post/publish.
func (h *Handlers) HandlePublish(w http.ResponseWriter, r *http.Request) {
	var req PublishRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	sourceID := h.sa(r)

	// Load account with decrypted credentials.
	account, err := h.loadAccountWithCredentials(r.Context(), req.AccountID, sourceID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if account == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "account not found"})
		return
	}

	tags := req.Tags
	if tags == nil {
		tags = []string{}
	}

	now := time.Now().UTC()
	isImmediate := req.ScheduleAt == nil || !req.ScheduleAt.After(now)

	initialStatus := "queued"
	if isImmediate {
		initialStatus = "publishing"
	}

	var entryID string
	var createdAt time.Time
	err = h.DB.QueryRow(r.Context(),
		`INSERT INTO np_post_queue
		 (source_account_id, platform, account_id, title, content, tags, schedule_at, status)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id, created_at`,
		sourceID, account.Platform, req.AccountID,
		req.Title, req.Content, tags, req.ScheduleAt, initialStatus,
	).Scan(&entryID, &createdAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if !isImmediate {
		writeJSON(w, http.StatusOK, PublishResponse{ID: entryID, Status: "queued"})
		return
	}

	// Dispatch to platform.
	result, publishErr := dispatchToPlatform(r.Context(), account, req.Title, req.Content, tags)
	if publishErr != nil {
		errJSON, _ := json.Marshal(map[string]string{"error": publishErr.Error()})
		h.DB.Exec(r.Context(), //nolint:errcheck
			`UPDATE np_post_queue SET status = 'failed', result = $1 WHERE id = $2`,
			string(errJSON), entryID,
		)
		// Distinguish "platform not implemented yet" (501) from a real
		// dispatch failure (502). The former is an API contract signal; the
		// latter is an upstream problem the caller may retry.
		var notImpl *platformNotImplementedError
		if errors.As(publishErr, &notImpl) {
			writeJSON(w, http.StatusNotImplemented, map[string]interface{}{
				"error":    "platform_not_implemented",
				"platform": notImpl.Platform,
				"message":  notImpl.Error(),
				"id":       entryID,
				"roadmap":  "https://github.com/nself-org/plugins-pro/issues",
			})
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]interface{}{
			"error":   "dispatch_failed",
			"message": publishErr.Error(),
			"id":      entryID,
		})
		return
	}

	resultJSON, _ := json.Marshal(result)
	h.DB.Exec(r.Context(), //nolint:errcheck
		`UPDATE np_post_queue SET status = 'published', result = $1 WHERE id = $2`,
		string(resultJSON), entryID,
	)
	writeJSON(w, http.StatusOK, PublishResponse{
		ID:     entryID,
		Status: "published",
		URL:    result.URL,
	})
}

// =============================================================================
// Queue
// =============================================================================

// HandleListQueue handles GET /post/queue?status=<optional>.
func (h *Handlers) HandleListQueue(w http.ResponseWriter, r *http.Request) {
	sourceID := h.sa(r)
	statusFilter := r.URL.Query().Get("status")

	var querySQL string
	var args []interface{}

	if statusFilter != "" {
		querySQL = `SELECT id, source_account_id, platform, account_id, title, content, tags,
		            schedule_at, status, result, created_at
		            FROM np_post_queue
		            WHERE source_account_id = $1 AND status = $2
		            ORDER BY created_at DESC`
		args = []interface{}{sourceID, statusFilter}
	} else {
		querySQL = `SELECT id, source_account_id, platform, account_id, title, content, tags,
		            schedule_at, status, result, created_at
		            FROM np_post_queue
		            WHERE source_account_id = $1
		            ORDER BY created_at DESC`
		args = []interface{}{sourceID}
	}

	rows, err := h.DB.Query(r.Context(), querySQL, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	entries := make([]QueueEntry, 0)
	for rows.Next() {
		var e QueueEntry
		var resultRaw *string
		if err := rows.Scan(
			&e.ID, &e.SourceAccountID, &e.Platform, &e.AccountID,
			&e.Title, &e.Content, &e.Tags,
			&e.ScheduleAt, &e.Status, &resultRaw, &e.CreatedAt,
		); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if resultRaw != nil {
			json.Unmarshal([]byte(*resultRaw), &e.Result) //nolint:errcheck
		}
		entries = append(entries, e)
	}
	writeJSON(w, http.StatusOK, entries)
}
