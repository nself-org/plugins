package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/nself-org/plugins-pro/paid/audit-analytics/go/patterns"
)

// RegisterReviewRoutes adds the privileged action review queue routes to mux,
// gated by the shared secret.
func (h *Handler) RegisterReviewRoutes(mux *http.ServeMux) {
	secret := SharedSecret()
	mux.HandleFunc("GET /audit/privileged-reviews", sharedSecretMiddleware(secret, h.listPendingReviews))
	mux.HandleFunc("POST /audit/privileged-reviews/{id}", sharedSecretMiddleware(secret, h.annotateReview))
	mux.HandleFunc("GET /audit/privileged-reviews/overdue", sharedSecretMiddleware(secret, h.listOverdueReviews))
}

func (h *Handler) listPendingReviews(w http.ResponseWriter, r *http.Request) {
	reviews, err := patterns.ListPendingReviews(r.Context(), h.query, tenantIDFromRequest(r))
	if err != nil {
		jsonError(w, "query failed", http.StatusInternalServerError)
		return
	}
	jsonOK(w, reviews)
}

func (h *Handler) listOverdueReviews(w http.ResponseWriter, r *http.Request) {
	reviews, err := patterns.ListOverdueReviews(r.Context(), h.query, tenantIDFromRequest(r))
	if err != nil {
		jsonError(w, "query failed", http.StatusInternalServerError)
		return
	}
	jsonOK(w, reviews)
}

func (h *Handler) annotateReview(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var body struct {
		ReviewerID string `json:"reviewer_id"`
		Notes      string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if body.ReviewerID == "" {
		jsonError(w, "reviewer_id required", http.StatusBadRequest)
		return
	}

	if err := patterns.AnnotateReview(r.Context(), h.exec, id, body.ReviewerID, body.Notes, tenantIDFromRequest(r)); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	jsonOK(w, map[string]bool{"ok": true})
}
