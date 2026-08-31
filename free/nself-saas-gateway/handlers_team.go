package main

// handlers_team.go — /v1/team: seats (owner + invited members).
//
// Purpose: the SPA's Team screen. The tenant owner (np_saas_tenants) is
//   always seat #1 and never a np_saas_team_members row; invited members are
//   rows there. Invites email a welcome message carrying a one-time join
//   token (only its SHA-256 is stored).
// Routes:  GET    /v1/team      — owner + members
//          POST   /v1/team      — invite by email (seat-limited by tier)
//          DELETE /v1/team/{id} — remove a member (never the owner)
// Outputs: {"members":[...]}, {"member":{...},"invite_sent":bool}.
// Constraints — P0 tenancy: every query is scoped by the verified tenant;
//   cross-tenant member ids 404. SEAT QUOTA (PRD §3): 1 + members >=
//   Limits.Seats → 402. Email failures never fail the invite (the row
//   exists; "invite_sent":false tells the SPA to surface a warning).

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/nself-org/plugins-pro/paid/shared/email"
	"github.com/nself-org/plugins-pro/paid/shared/saas"
)

// teamMemberDTO is the contract TeamMember shape. The owner appears with
// id "owner", role "owner", status "active".
type teamMemberDTO struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Name      string `json:"name,omitempty"`
	Role      string `json:"role"`   // owner | member
	Status    string `json:"status"` // active | invited
	InvitedAt string `json:"invited_at,omitempty"`
	JoinedAt  string `json:"joined_at,omitempty"`
}

// newInviteToken mints a 32-byte hex join token + its SHA-256 hex digest.
func newInviteToken() (raw, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	raw = hex.EncodeToString(buf)
	sum := sha256.Sum256([]byte(raw))
	return raw, hex.EncodeToString(sum[:]), nil
}

// ownerMember loads the tenant owner as the synthetic first seat.
func (g *gateway) ownerMember(ctx context.Context, tenantID string) (teamMemberDTO, error) {
	m := teamMemberDTO{ID: "owner", Role: "owner", Status: "active"}
	var email, name sql.NullString
	err := g.db.QueryRowContext(ctx,
		`SELECT owner_email, owner_name FROM np_saas_tenants WHERE tenant_id = $1`,
		tenantID).Scan(&email, &name)
	if err != nil {
		return m, err
	}
	m.Email = email.String
	m.Name = name.String
	return m, nil
}

// handleListTeam — GET /v1/team.
func (g *gateway) handleListTeam(w http.ResponseWriter, r *http.Request) {
	tenantID := saas.TenantFrom(r.Context())

	owner, err := g.ownerMember(r.Context(), tenantID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "owner lookup failed")
		return
	}
	members := []teamMemberDTO{owner}

	rows, err := g.db.QueryContext(r.Context(), `
		SELECT id::text, email, name, role, status, invited_at, joined_at
		FROM np_saas_team_members
		WHERE tenant_id = $1
		ORDER BY invited_at ASC`, tenantID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "team lookup failed")
		return
	}
	defer rows.Close() //nolint:errcheck
	for rows.Next() {
		var m teamMemberDTO
		var invitedAt time.Time
		var joinedAt sql.NullTime
		if err := rows.Scan(&m.ID, &m.Email, &m.Name, &m.Role, &m.Status, &invitedAt, &joinedAt); err != nil {
			writeErr(w, http.StatusInternalServerError, "db_error", "team scan failed")
			return
		}
		m.InvitedAt = invitedAt.UTC().Format(time.RFC3339)
		if joinedAt.Valid {
			m.JoinedAt = joinedAt.Time.UTC().Format(time.RFC3339)
		}
		members = append(members, m)
	}
	if rows.Err() != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "team iteration failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": members})
}

// handleInviteTeamMember — POST /v1/team {"email","name"?}.
func (g *gateway) handleInviteTeamMember(w http.ResponseWriter, r *http.Request) {
	tenantID := saas.TenantFrom(r.Context())

	var req struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid JSON: "+err.Error())
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.Name = strings.TrimSpace(req.Name)
	if _, err := mail.ParseAddress(req.Email); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "invalid_request", "a valid email is required")
		return
	}

	// SEAT QUOTA: owner (1) + existing members must stay under Limits.Seats.
	limits, tier, found, err := saas.EffectiveLimits(r.Context(), g.db, tenantID)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, "quota_unavailable", "quota check unavailable")
		return
	}
	if found {
		var members int64
		if err := g.db.QueryRowContext(r.Context(),
			`SELECT COUNT(*) FROM np_saas_team_members WHERE tenant_id = $1`,
			tenantID).Scan(&members); err != nil {
			writeErr(w, http.StatusServiceUnavailable, "quota_unavailable", "quota check unavailable")
			return
		}
		if 1+members >= int64(limits.Seats) {
			writeErr(w, http.StatusPaymentRequired, "quota_exceeded",
				"your "+string(tier)+" tier includes "+itoa(limits.Seats)+
					" seat(s) and all are in use. Upgrade at "+saas.UpgradeURL+".")
			return
		}
	}

	rawToken, tokenHash, err := newInviteToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "token_error", "invite token generation failed")
		return
	}

	var m teamMemberDTO
	var invitedAt time.Time
	err = g.db.QueryRowContext(r.Context(), `
		INSERT INTO np_saas_team_members (tenant_id, email, name, invite_token_hash)
		VALUES ($1, $2, $3, $4)
		RETURNING id::text, email, name, role, status, invited_at`,
		tenantID, req.Email, req.Name, tokenHash).
		Scan(&m.ID, &m.Email, &m.Name, &m.Role, &m.Status, &invitedAt)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			writeErr(w, http.StatusConflict, "already_invited",
				"this email is already on the team")
			return
		}
		writeErr(w, http.StatusInternalServerError, "db_error", "invite create failed")
		return
	}
	m.InvitedAt = invitedAt.UTC().Format(time.RFC3339)

	// Welcome email with the one-time join link. Best-effort: the invite row
	// exists either way; the SPA surfaces invite_sent=false as a warning.
	inviteSent := false
	if g.mail != nil {
		joinURL := g.cfg.DashboardBaseURL + "/join?token=" + rawToken
		if err := g.mail.Send(r.Context(), req.Email, email.TemplateWelcome, map[string]string{
			"Name": firstNonEmpty(req.Name, req.Email),
			"URL":  joinURL,
		}); err != nil {
			log.Printf("saas-gateway: invite email to %s: %v", req.Email, err)
		} else {
			inviteSent = true
		}
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"member":      m,
		"invite_sent": inviteSent,
	})
}

// handleRemoveTeamMember — DELETE /v1/team/{id}. The owner is not a member
// row, so it can never be removed here.
func (g *gateway) handleRemoveTeamMember(w http.ResponseWriter, r *http.Request) {
	tenantID := saas.TenantFrom(r.Context())
	id := chi.URLParam(r, "id")
	if id == "owner" {
		writeErr(w, http.StatusUnprocessableEntity, "invalid_request",
			"the account owner cannot be removed")
		return
	}
	if uuid.Validate(id) != nil {
		writeErr(w, http.StatusNotFound, "not_found", "team member not found")
		return
	}
	res, err := g.db.ExecContext(r.Context(),
		`DELETE FROM np_saas_team_members WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "member delete failed")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, http.StatusNotFound, "not_found", "team member not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

// firstNonEmpty returns the first non-empty string.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
