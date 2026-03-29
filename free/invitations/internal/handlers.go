package internal

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	sdk "github.com/nself-org/plugin-sdk"
)

// RegisterRoutes mounts all invitation endpoints on the given router.
func RegisterRoutes(r chi.Router, pool *pgxpool.Pool) {
	r.Post("/v1/invitations", handleCreateInvitation(pool))
	r.Get("/v1/invitations", handleListInvitations(pool))
	r.Get("/v1/invitations/{id}", handleGetInvitation(pool))
	r.Delete("/v1/invitations/{id}", handleRevokeInvitation(pool))
	r.Post("/v1/invitations/{token}/accept", handleAcceptInvitation(pool))
	r.Post("/v1/invitations/{id}/resend", handleResendInvitation(pool))
}

// --- Request / Response types ------------------------------------------------

// CreateInvitationRequest is the JSON body for POST /v1/invitations.
type CreateInvitationRequest struct {
	Email     string `json:"email"`
	Role      string `json:"role"`
	ExpiresAt string `json:"expires_at"` // RFC3339 timestamp, optional
}

// --- Handlers ----------------------------------------------------------------

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func handleCreateInvitation(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateInvitationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}

		if req.Email == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "email is required"})
			return
		}

		role := req.Role
		if role == "" {
			role = "member"
		}

		token, err := generateToken()
		if err != nil {
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate token"})
			return
		}

		var expiresAt *time.Time
		if req.ExpiresAt != "" {
			t, err := time.Parse(time.RFC3339, req.ExpiresAt)
			if err != nil {
				sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "expires_at must be RFC3339 format"})
				return
			}
			expiresAt = &t
		}

		inv := &Invitation{
			ID:        uuid.New().String(),
			Email:     req.Email,
			Role:      role,
			Token:     token,
			Status:    "pending",
			InvitedBy: r.Header.Get("X-User-ID"),
			ExpiresAt: expiresAt,
		}

		if inv.InvitedBy == "" {
			inv.InvitedBy = "system"
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		if err := InsertInvitation(ctx, pool, inv); err != nil {
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": "failed to create invitation: " + err.Error()})
			return
		}

		sdk.Respond(w, http.StatusCreated, map[string]interface{}{
			"id":         inv.ID,
			"email":      inv.Email,
			"role":       inv.Role,
			"token":      inv.Token,
			"status":     inv.Status,
			"invited_by": inv.InvitedBy,
			"expires_at": inv.ExpiresAt,
		})
	}
}

func handleListInvitations(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := r.URL.Query().Get("status")

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

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		invitations, err := ListInvitations(ctx, pool, status, limit, offset)
		if err != nil {
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		if invitations == nil {
			invitations = []Invitation{}
		}

		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"invitations": invitations,
			"limit":       limit,
			"offset":      offset,
		})
	}
}

func handleGetInvitation(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		inv, err := GetInvitation(ctx, pool, id)
		if err != nil {
			sdk.Respond(w, http.StatusNotFound, map[string]string{"error": "invitation not found"})
			return
		}

		sdk.Respond(w, http.StatusOK, inv)
	}
}

func handleRevokeInvitation(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		if err := RevokeInvitation(ctx, pool, id); err != nil {
			sdk.Respond(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}

		sdk.Respond(w, http.StatusOK, map[string]string{"status": "revoked"})
	}
}

func handleAcceptInvitation(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := chi.URLParam(r, "token")
		if token == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "token is required"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		// Check if the invitation exists and is still valid.
		existing, err := GetInvitationByToken(ctx, pool, token)
		if err != nil {
			sdk.Respond(w, http.StatusNotFound, map[string]string{"error": "invitation not found"})
			return
		}

		if existing.Status != "pending" {
			sdk.Respond(w, http.StatusConflict, map[string]string{"error": "invitation is not pending"})
			return
		}

		if existing.ExpiresAt != nil && existing.ExpiresAt.Before(time.Now().UTC()) {
			sdk.Respond(w, http.StatusGone, map[string]string{"error": "invitation has expired"})
			return
		}

		inv, err := AcceptInvitation(ctx, pool, token)
		if err != nil {
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": "failed to accept invitation: " + err.Error()})
			return
		}

		sdk.Respond(w, http.StatusOK, inv)
	}
}

func handleResendInvitation(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if id == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		inv, err := GetInvitation(ctx, pool, id)
		if err != nil {
			sdk.Respond(w, http.StatusNotFound, map[string]string{"error": "invitation not found"})
			return
		}

		if inv.Status != "pending" {
			sdk.Respond(w, http.StatusConflict, map[string]string{"error": "only pending invitations can be resent"})
			return
		}

		// Update the updated_at timestamp to indicate a resend.
		if err := UpdateInvitationStatus(ctx, pool, id, "pending"); err != nil {
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": "failed to update invitation: " + err.Error()})
			return
		}

		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"id":     inv.ID,
			"email":  inv.Email,
			"token":  inv.Token,
			"status": "resent",
		})
	}
}
