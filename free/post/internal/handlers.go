package internal

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nself-org/nself-post/internal/providers/devto"
	"github.com/nself-org/nself-post/internal/providers/ghost"
	"github.com/nself-org/nself-post/internal/providers/hashnode"
	"github.com/nself-org/nself-post/internal/providers/linkedin"
	"github.com/nself-org/nself-post/internal/providers/telegram"
	"github.com/nself-org/nself-post/internal/providers/twitter"
	"github.com/nself-org/nself-post/internal/providers/wordpress"
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
// Encryption helpers
// =============================================================================

// encryptCredentials encrypts plaintext using AES-256-GCM and returns a
// base64-encoded nonce||ciphertext string.
func (h *Handlers) encryptCredentials(plaintext []byte) (string, error) {
	block, err := aes.NewCipher(h.Key[:])
	if err != nil {
		return "", fmt.Errorf("new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("new gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("nonce: %w", err)
	}
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decryptCredentials decrypts a base64-encoded nonce||ciphertext blob.
func (h *Handlers) decryptCredentials(encoded string) ([]byte, error) {
	combined, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}
	block, err := aes.NewCipher(h.Key[:])
	if err != nil {
		return nil, fmt.Errorf("new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(combined) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, ct := combined[:nonceSize], combined[nonceSize:]
	return gcm.Open(nil, nonce, ct, nil)
}

// =============================================================================
// Health
// =============================================================================

// HandleHealth returns a simple liveness check.
func (h *Handlers) HandleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"service": "nself-post",
	})
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

// =============================================================================
// Internal helpers
// =============================================================================

// loadAccountWithCredentials fetches an account and decrypts its credentials.
// Returns (nil, nil) when the account does not exist.
func (h *Handlers) loadAccountWithCredentials(ctx context.Context, accountID, sourceID string) (*PostAccount, error) {
	var a PostAccount
	var credRaw string
	err := h.DB.QueryRow(ctx,
		`SELECT id, source_account_id, platform, name, endpoint, credentials, created_at
		 FROM np_post_accounts WHERE id = $1 AND source_account_id = $2`,
		accountID, sourceID,
	).Scan(&a.ID, &a.SourceAccountID, &a.Platform, &a.Name, &a.Endpoint, &credRaw, &a.CreatedAt)
	if err != nil {
		return nil, nil //nolint:nilerr — treat not-found and other scan errors as "not found"
	}

	var credMap map[string]interface{}
	if err := json.Unmarshal([]byte(credRaw), &credMap); err == nil {
		if enc, ok := credMap["__enc"].(string); ok {
			plain, err := h.decryptCredentials(enc)
			if err != nil {
				return nil, fmt.Errorf("decrypt credentials: %w", err)
			}
			if err := json.Unmarshal(plain, &credMap); err != nil {
				return nil, fmt.Errorf("unmarshal credentials: %w", err)
			}
		}
	}
	a.Credentials = credMap
	return &a, nil
}

// platformDispatchResult is the outcome of a publish operation.
type platformDispatchResult struct {
	URL      *string
	PostID   *string
	Metadata map[string]interface{}
}

// platformNotImplementedError is returned by dispatchToPlatform when the
// caller requested a platform we have not yet shipped an adapter for. Handlers
// use errors.As to detect this and surface a proper 501 + roadmap URL.
type platformNotImplementedError struct {
	Platform string
}

func (e *platformNotImplementedError) Error() string {
	return fmt.Sprintf("platform %q is not yet implemented — see roadmap at https://github.com/nself-org/plugins-pro/issues", e.Platform)
}

// dispatchToPlatform routes the publish request to the appropriate platform
// adapter.
//
// Shipped adapters: internal (no-op queue row), wordpress (XML-RPC wp.newPost),
// ghost (Admin API). Everything else returns *platformNotImplementedError —
// the caller uses errors.As to map that to HTTP 501. Real dispatch failures
// (network, auth, 4xx/5xx from the upstream) are returned as wrapped errors
// and mapped to HTTP 502 by the caller.
func dispatchToPlatform(ctx context.Context, account *PostAccount, title, content string, tags []string) (*platformDispatchResult, error) {
	switch account.Platform {
	case "internal":
		// Internal posts are stored in np_post_queue by the caller. The dispatch
		// is a no-op success — the queue row IS the published artifact.
		return &platformDispatchResult{URL: nil}, nil

	case "wordpress":
		return dispatchWordPress(ctx, account, title, content, tags)

	case "ghost":
		return dispatchGhost(ctx, account, title, content, tags)

	case "twitter", "x":
		return dispatchTwitter(ctx, account, title, content, tags)
	case "linkedin":
		return dispatchLinkedIn(ctx, account, title, content, tags)
	case "telegram":
		return dispatchTelegram(ctx, account, title, content, tags)
	case "devto", "dev.to":
		return dispatchDevTo(ctx, account, title, content, tags)
	case "hashnode":
		return dispatchHashnode(ctx, account, title, content, tags)
	case "mastodon":
		return nil, &platformNotImplementedError{Platform: "mastodon"}
	case "bluesky":
		return nil, &platformNotImplementedError{Platform: "bluesky"}
	case "facebook":
		return nil, &platformNotImplementedError{Platform: "facebook"}
	case "instagram":
		return nil, &platformNotImplementedError{Platform: "instagram"}
	case "threads":
		return nil, &platformNotImplementedError{Platform: "threads"}

	default:
		return nil, fmt.Errorf("unknown platform %q — supported: internal, wordpress, ghost; planned: twitter, linkedin, mastodon, bluesky, facebook, instagram, threads", account.Platform)
	}
}

// dispatchWordPress publishes to a WordPress site via XML-RPC. Required
// credential fields on the account:
//   - endpoint: site root or full xmlrpc.php URL (falls back to account.Endpoint)
//   - username: WP username
//   - app_password: WP Application Password
// Optional:
//   - categories: []string
//   - status: "publish" (default), "draft", "private", "pending"
func dispatchWordPress(ctx context.Context, account *PostAccount, title, content string, tags []string) (*platformDispatchResult, error) {
	endpoint := stringCred(account.Credentials, "endpoint")
	if endpoint == "" && account.Endpoint != nil {
		endpoint = *account.Endpoint
	}
	username := stringCred(account.Credentials, "username")
	appPass := stringCred(account.Credentials, "app_password")
	if appPass == "" {
		// Allow the common spelling "application_password" as well.
		appPass = stringCred(account.Credentials, "application_password")
	}
	if endpoint == "" || username == "" || appPass == "" {
		return nil, fmt.Errorf("wordpress credentials incomplete: require endpoint, username, app_password")
	}
	categories := stringsCred(account.Credentials, "categories")
	status := stringCred(account.Credentials, "status")

	client := wordpress.NewClient(endpoint, username, appPass)
	res, err := client.NewPost(ctx, wordpress.NewPostArgs{
		Title:      title,
		Content:    content,
		Categories: categories,
		Tags:       tags,
		Status:     status,
	})
	if err != nil {
		return nil, err
	}
	out := &platformDispatchResult{
		Metadata: map[string]interface{}{"post_id": res.PostID},
	}
	if res.PostID != "" {
		id := res.PostID
		out.PostID = &id
	}
	if res.Permalink != "" {
		link := res.Permalink
		out.URL = &link
	}
	return out, nil
}

// dispatchGhost publishes to a Ghost site via the Admin API. Required
// credential fields:
//   - endpoint: site root (falls back to account.Endpoint)
//   - admin_api_key: "<keyId>:<hexSecret>"
// Optional:
//   - status: "published" (default), "draft", "scheduled"
func dispatchGhost(ctx context.Context, account *PostAccount, title, content string, tags []string) (*platformDispatchResult, error) {
	endpoint := stringCred(account.Credentials, "endpoint")
	if endpoint == "" && account.Endpoint != nil {
		endpoint = *account.Endpoint
	}
	key := stringCred(account.Credentials, "admin_api_key")
	if endpoint == "" || key == "" {
		return nil, fmt.Errorf("ghost credentials incomplete: require endpoint, admin_api_key")
	}
	status := stringCred(account.Credentials, "status")

	client := ghost.NewClient(endpoint, key)
	res, err := client.NewPost(ctx, ghost.NewPostArgs{
		Title:  title,
		HTML:   content,
		Tags:   tags,
		Status: status,
	})
	if err != nil {
		return nil, err
	}
	out := &platformDispatchResult{
		Metadata: map[string]interface{}{"post_id": res.PostID},
	}
	if res.PostID != "" {
		id := res.PostID
		out.PostID = &id
	}
	if res.URL != "" {
		url := res.URL
		out.URL = &url
	}
	return out, nil
}

// stringCred reads a string field from the decrypted credentials map. Returns
// "" when the key is missing or of the wrong type.
func stringCred(creds map[string]interface{}, key string) string {
	if creds == nil {
		return ""
	}
	if v, ok := creds[key].(string); ok {
		return v
	}
	return ""
}

// stringsCred reads a []string field from the decrypted credentials map.
// Accepts both []string (unlikely after JSON round-trip) and []interface{}
// (the normal case after json.Unmarshal).
func stringsCred(creds map[string]interface{}, key string) []string {
	if creds == nil {
		return nil
	}
	switch v := creds[key].(type) {
	case []string:
		return v
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// dispatchTwitter publishes via Twitter/X v2 API.
// Required credential fields: bearer_token.
func dispatchTwitter(ctx context.Context, account *PostAccount, title, content string, tags []string) (*platformDispatchResult, error) {
	token := stringCred(account.Credentials, "bearer_token")
	if token == "" {
		return nil, fmt.Errorf("twitter credentials incomplete: require bearer_token")
	}
	text := title
	if content != "" {
		text = title + "\n\n" + content
	}
	client := twitter.NewClient(token)
	res, err := client.Post(ctx, twitter.PostArgs{Text: text})
	if err != nil {
		return nil, err
	}
	out := &platformDispatchResult{Metadata: map[string]interface{}{"tweet_id": res.TweetID}}
	if res.URL != "" {
		url := res.URL
		out.URL = &url
	}
	return out, nil
}

// dispatchLinkedIn publishes via LinkedIn UGC Posts API.
// Required credential fields: access_token. Optional: author_urn.
func dispatchLinkedIn(ctx context.Context, account *PostAccount, title, content string, tags []string) (*platformDispatchResult, error) {
	token := stringCred(account.Credentials, "access_token")
	if token == "" {
		return nil, fmt.Errorf("linkedin credentials incomplete: require access_token")
	}
	authorURN := stringCred(account.Credentials, "author_urn")
	text := title
	if content != "" {
		text = title + "\n\n" + content
	}
	client := linkedin.NewClient(token)
	res, err := client.Post(ctx, linkedin.PostArgs{AuthorURN: authorURN, Text: text})
	if err != nil {
		return nil, err
	}
	out := &platformDispatchResult{Metadata: map[string]interface{}{"post_urn": res.PostURN}}
	if res.URL != "" {
		url := res.URL
		out.URL = &url
	}
	return out, nil
}

// dispatchTelegram publishes via Telegram Bot API.
// Required credential fields: bot_token, chat_id.
func dispatchTelegram(ctx context.Context, account *PostAccount, title, content string, tags []string) (*platformDispatchResult, error) {
	botToken := stringCred(account.Credentials, "bot_token")
	chatID := stringCred(account.Credentials, "chat_id")
	if account.Endpoint != nil && chatID == "" {
		chatID = *account.Endpoint
	}
	if botToken == "" || chatID == "" {
		return nil, fmt.Errorf("telegram credentials incomplete: require bot_token, chat_id")
	}
	text := title
	if content != "" {
		text = "<b>" + title + "</b>\n\n" + content
	}
	client := telegram.NewClient(botToken)
	res, err := client.Post(ctx, telegram.PostArgs{ChatID: chatID, Text: text})
	if err != nil {
		return nil, err
	}
	out := &platformDispatchResult{Metadata: map[string]interface{}{"message_id": res.MessageID}}
	if res.URL != "" {
		url := res.URL
		out.URL = &url
	}
	return out, nil
}

// dispatchDevTo publishes via Dev.to Articles API.
// Required credential fields: api_key.
func dispatchDevTo(ctx context.Context, account *PostAccount, title, content string, tags []string) (*platformDispatchResult, error) {
	apiKey := stringCred(account.Credentials, "api_key")
	if apiKey == "" {
		return nil, fmt.Errorf("devto credentials incomplete: require api_key")
	}
	client := devto.NewClient(apiKey)
	res, err := client.Post(ctx, devto.PostArgs{
		Title:     title,
		Content:   content,
		Tags:      tags,
		Published: true,
	})
	if err != nil {
		return nil, err
	}
	out := &platformDispatchResult{Metadata: map[string]interface{}{"article_id": res.ArticleID}}
	if res.URL != "" {
		url := res.URL
		out.URL = &url
	}
	return out, nil
}

// dispatchHashnode publishes via Hashnode GraphQL API.
// Required credential fields: api_key, publication_host.
func dispatchHashnode(ctx context.Context, account *PostAccount, title, content string, tags []string) (*platformDispatchResult, error) {
	apiKey := stringCred(account.Credentials, "api_key")
	pubHost := stringCred(account.Credentials, "publication_host")
	if account.Endpoint != nil && pubHost == "" {
		pubHost = *account.Endpoint
	}
	if apiKey == "" || pubHost == "" {
		return nil, fmt.Errorf("hashnode credentials incomplete: require api_key, publication_host")
	}
	client := hashnode.NewClient(apiKey, pubHost)
	res, err := client.Post(ctx, hashnode.PostArgs{
		Title:   title,
		Content: content,
		Tags:    tags,
	})
	if err != nil {
		return nil, err
	}
	out := &platformDispatchResult{Metadata: map[string]interface{}{"post_id": res.PostID}}
	if res.URL != "" {
		url := res.URL
		out.URL = &url
	}
	return out, nil
}
