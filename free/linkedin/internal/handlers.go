package internal

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

)

// Handlers holds shared dependencies for all HTTP handlers.
type Handlers struct {
	Cfg  *Config
	Pool dbPool
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

// sourceAccount returns the source account ID from the X-Source-Account-ID
// header, falling back to "primary".
func sourceAccount(r *http.Request) string {
	if v := r.Header.Get("X-Source-Account-ID"); v != "" {
		return v
	}
	return "primary"
}

// generateState creates a cryptographically random state token for OAuth.
func generateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("rand: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// =============================================================================
// Health
// =============================================================================

// HandleHealth returns a simple liveness check.
func (h *Handlers) HandleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"service": "nself-linkedin",
	})
}

// =============================================================================
// OAuth — T-7611
// =============================================================================

// HandleAuthStart handles GET /linkedin/auth/start.
// Redirects the user to LinkedIn's authorization page.
func (h *Handlers) HandleAuthStart(w http.ResponseWriter, r *http.Request) {
	state, err := generateState()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not generate state"})
		return
	}

	// Store state in a short-lived cookie so we can validate it in the callback.
	http.SetCookie(w, &http.Cookie{
		Name:     "li_oauth_state",
		Value:    state,
		Path:     "/",
		MaxAge:   600, // 10 minutes
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	authURL := OAuthURL(h.Cfg.ClientID, h.Cfg.RedirectURI, state)
	http.Redirect(w, r, authURL, http.StatusFound)
}

// HandleAuthCallback handles GET /linkedin/auth/callback.
// Exchanges the authorization code for an access token and stores it in DB.
func (h *Handlers) HandleAuthCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	// Validate state to prevent CSRF.
	stateCookie, err := r.Cookie("li_oauth_state")
	if err != nil || subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(stateCookie.Value)) != 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid state"})
		return
	}

	code := q.Get("code")
	if code == "" {
		errMsg := q.Get("error_description")
		if errMsg == "" {
			errMsg = q.Get("error")
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no code received: " + errMsg})
		return
	}

	// Exchange code for token.
	accessToken, expiresAt, err := ExchangeCode(r.Context(),
		h.Cfg.ClientID, h.Cfg.ClientSecret, h.Cfg.RedirectURI, code)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}

	// Fetch profile to get LinkedIn member ID and name.
	linkedInID, name, email, err := FetchProfile(r.Context(), accessToken)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "profile fetch: " + err.Error()})
		return
	}

	sourceID := sourceAccount(r)

	// Upsert token in DB.
	if err := h.Pool.upsertToken(r.Context(), sourceID, accessToken, expiresAt, linkedInID, name, email); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Clear the state cookie.
	http.SetCookie(w, &http.Cookie{
		Name:   "li_oauth_state",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"connected":   true,
		"name":        name,
		"linkedin_id": linkedInID,
		"expires_at":  expiresAt,
	})
}

// =============================================================================
// Status — T-7614
// =============================================================================

// HandleStatus handles GET /linkedin/status.
func (h *Handlers) HandleStatus(w http.ResponseWriter, r *http.Request) {
	sourceID := sourceAccount(r)

	tok, err := h.Pool.loadToken(r.Context(), sourceID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if tok == nil {
		writeJSON(w, http.StatusOK, StatusResponse{Connected: false})
		return
	}

	writeJSON(w, http.StatusOK, StatusResponse{
		Connected: true,
		Name:      tok.Name,
		Email:     tok.Email,
		ExpiresAt: tok.ExpiresAt,
	})
}

// =============================================================================
// Publish — T-7612
// =============================================================================

// HandlePublish handles POST /linkedin/publish.
func (h *Handlers) HandlePublish(w http.ResponseWriter, r *http.Request) {
	var req PublishRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if req.Text == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "text is required"})
		return
	}

	if req.Visibility == "" {
		req.Visibility = "PUBLIC"
	}
	if req.Visibility != "PUBLIC" && req.Visibility != "CONNECTIONS" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "visibility must be PUBLIC or CONNECTIONS"})
		return
	}

	sourceID := sourceAccount(r)

	// Load token.
	tok, err := h.Pool.loadToken(r.Context(), sourceID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if tok == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "LinkedIn account not connected"})
		return
	}
	if time.Now().UTC().After(tok.ExpiresAt) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "LinkedIn token expired; please reconnect"})
		return
	}

	// Publish to LinkedIn.
	linkedInPostID, publishErr := PublishPost(
		r.Context(), tok.AccessToken, tok.LinkedInID,
		req.Text, req.Visibility, req.ImageURL,
	)

	// Persist post record regardless of outcome so history is accurate.
	status := "published"
	var errMsg *string
	if publishErr != nil {
		status = "failed"
		msg := publishErr.Error()
		errMsg = &msg
	}

	var postID string
	var publishedAt time.Time
	var storedPostID *string
	if linkedInPostID != "" {
		storedPostID = &linkedInPostID
	}
	postID, publishedAt, err = h.Pool.insertPost(r.Context(),
		sourceID, storedPostID, req.Text, req.ImageURL, req.Visibility, status, errMsg)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "store post: " + err.Error()})
		return
	}
	_ = publishedAt

	if publishErr != nil {
		writeJSON(w, http.StatusBadGateway, map[string]interface{}{
			"error": publishErr.Error(),
			"id":    postID,
			"status": "failed",
		})
		return
	}

	writeJSON(w, http.StatusOK, PublishResponse{
		ID:             postID,
		LinkedInPostID: storedPostID,
		Status:         "published",
	})
}

// =============================================================================
// Posts history — T-7615
// =============================================================================

// HandleListPosts handles GET /linkedin/posts.
func (h *Handlers) HandleListPosts(w http.ResponseWriter, r *http.Request) {
	sourceID := sourceAccount(r)

	posts, err := h.Pool.listPosts(r.Context(), sourceID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, posts)
}

// =============================================================================
// Internal tools descriptor — T-7613
// =============================================================================

// HandleTools handles GET /internal/tools.
// Returns the Claw tool descriptor for linkedin.publish_post.
func (h *Handlers) HandleTools(w http.ResponseWriter, _ *http.Request) {
	tools := []ToolDescriptor{
		{
			Name:        "linkedin.publish_post",
			Description: "Publish a text post (optionally with an image URL) to the connected LinkedIn account.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"text": map[string]interface{}{
						"type":        "string",
						"description": "The text content of the LinkedIn post.",
					},
					"visibility": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"PUBLIC", "CONNECTIONS"},
						"description": "Who can see the post. Defaults to PUBLIC.",
					},
					"image_url": map[string]interface{}{
						"type":        "string",
						"description": "Optional URL of an image or article to attach.",
					},
				},
				"required": []string{"text"},
			},
		},
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"tools": tools})
}

// =============================================================================
// Auth middleware
// =============================================================================

// InternalAuthMiddleware rejects requests that do not carry the correct
// X-Internal-Token header.
func InternalAuthMiddleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := r.Header.Get("X-Internal-Token")
			if token == "" {
				http.Error(w, "X-Internal-Token required", http.StatusUnauthorized)
				return
			}
			if subtle.ConstantTimeCompare([]byte(token), []byte(secret)) != 1 {
				http.Error(w, "Invalid internal token", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// =============================================================================
// Disconnect
// =============================================================================

// HandleDisconnect handles DELETE /linkedin/auth — removes stored token.
func (h *Handlers) HandleDisconnect(w http.ResponseWriter, r *http.Request) {
	sourceID := sourceAccount(r)

	if err := h.Pool.deleteToken(r.Context(), sourceID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
