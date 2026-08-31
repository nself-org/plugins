package main

// handlers_join.go — team-invite ACCEPT: the consumer for the join tokens
// minted by POST /v1/team (handlers_team.go).
//
// Purpose: turn an emailed one-time join token into an ACTIVE team member
//   with a password login and a session JWT — the missing half of the invite
//   flow (G-GATEWAY minted + emailed tokens; nothing consumed them).
// Routes (PUBLIC — no session exists yet; per-IP rate-limited):
//   GET  /v1/join/{token}  → invite info (tenant name, email, role) for the
//        SPA's accept page. Token-gated only.
//   POST /v1/join          {"token","password","name"?} → validates the
//        token (SHA-256 match, invited status, not expired), sets the member
//        password (bcrypt), marks the token consumed (status=active,
//        joined_at, hash cleared), and returns a login JWT scoped to the
//        INVITING tenant with role "member".
// Constraints — P0 tenancy: the token IS the credential; the tenant comes
//   ONLY from the matched invite row (client input can never choose it).
//   ANTI-ENUMERATION: bad, expired, and consumed tokens are all the same
//   generic 404. SEAT QUOTA: re-checked at accept time (a tier downgrade
//   between invite and accept must not oversubscribe) — 402 when full.
//   Consumption is atomic (guarded UPDATE) so a raced double-accept loses.

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/nself-org/plugins-pro/paid/shared/saas"
)

// inviteTTLDays — join tokens expire this many days after the invite is
// created (the welcome email tells the invitee to act promptly; a fresh
// invite re-arms the flow after removal + re-add).
const inviteTTLDays = 14

// joinInvite is one valid (invited, unexpired) invite row + tenant context.
type joinInvite struct {
	MemberID   string
	TenantID   string
	Email      string
	Name       string
	Role       string
	TenantName string // owner display name, falling back to owner email
	Tier       string
}

// lookupInvite resolves a raw join token to its invite row. sql.ErrNoRows
// covers bad, expired, AND consumed tokens alike (anti-enumeration: callers
// answer all three with the same generic 404).
func (g *gateway) lookupInvite(ctx context.Context, rawToken string) (joinInvite, error) {
	var inv joinInvite
	var ownerName, ownerEmail sql.NullString
	err := g.db.QueryRowContext(ctx, `
		SELECT m.id::text, m.tenant_id::text, m.email, m.name, m.role,
		       t.owner_name, t.owner_email, t.tier
		FROM np_saas_team_members m
		JOIN np_saas_tenants t ON t.tenant_id = m.tenant_id
		WHERE m.invite_token_hash = $1
		  AND m.status = 'invited'
		  AND m.invited_at > NOW() - make_interval(days => $2)`,
		sha256Hex(rawToken), inviteTTLDays).
		Scan(&inv.MemberID, &inv.TenantID, &inv.Email, &inv.Name, &inv.Role,
			&ownerName, &ownerEmail, &inv.Tier)
	if err != nil {
		return joinInvite{}, err
	}
	inv.TenantName = firstNonEmpty(ownerName.String, ownerEmail.String)
	return inv, nil
}

// writeInviteNotFound is the single generic response for bad/expired/consumed
// tokens — indistinguishable on purpose.
func writeInviteNotFound(w http.ResponseWriter) {
	writeErr(w, http.StatusNotFound, "not_found", "invite not found or expired")
}

// handleJoinInfo — GET /v1/join/{token} (public, token-gated). Returns the
// invite context the SPA renders on the accept page.
func (g *gateway) handleJoinInfo(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(chi.URLParam(r, "token"))
	if token == "" {
		writeInviteNotFound(w)
		return
	}
	inv, err := g.lookupInvite(r.Context(), token)
	if errors.Is(err, sql.ErrNoRows) {
		writeInviteNotFound(w)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "invite lookup failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"invite": map[string]any{
			"tenant_name": inv.TenantName,
			"email":       inv.Email,
			"name":        inv.Name,
			"role":        inv.Role,
		},
	})
}

// handleJoinAccept — POST /v1/join {"token","password","name"?}.
func (g *gateway) handleJoinAccept(w http.ResponseWriter, r *http.Request) {
	if len(g.cfg.JWTSecret) == 0 {
		writeErr(w, http.StatusServiceUnavailable, "auth_disabled",
			"SAAS_JWT_HS256_SECRET is not configured")
		return
	}
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
		Name     string `json:"name"`
	}
	if !decodeFlowBody(w, r, &req) {
		return
	}
	if msg := validatePassword(req.Password); msg != "" {
		writeErr(w, http.StatusUnprocessableEntity, "invalid_request", msg)
		return
	}
	req.Token = strings.TrimSpace(req.Token)
	req.Name = strings.TrimSpace(req.Name)
	if req.Token == "" {
		writeInviteNotFound(w)
		return
	}

	inv, err := g.lookupInvite(r.Context(), req.Token)
	if errors.Is(err, sql.ErrNoRows) {
		writeInviteNotFound(w)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "invite lookup failed")
		return
	}

	// SEAT QUOTA at ACCEPT time: the owner (1) + already-ACTIVE members must
	// leave room for this member to activate (guards tier downgrades between
	// invite and accept).
	limits, tier, found, err := saas.EffectiveLimits(r.Context(), g.db, inv.TenantID)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, "quota_unavailable", "quota check unavailable")
		return
	}
	if found {
		var active int64
		if err := g.db.QueryRowContext(r.Context(),
			`SELECT COUNT(*) FROM np_saas_team_members WHERE tenant_id = $1 AND status = 'active'`,
			inv.TenantID).Scan(&active); err != nil {
			writeErr(w, http.StatusServiceUnavailable, "quota_unavailable", "quota check unavailable")
			return
		}
		if 1+active >= int64(limits.Seats) {
			writeErr(w, http.StatusPaymentRequired, "quota_exceeded",
				"this team's "+string(tier)+" tier includes "+itoa(limits.Seats)+
					" seat(s) and all are in use. The owner can upgrade at "+saas.UpgradeURL+".")
			return
		}
	}

	hash, err := hashPassword(req.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "hash_error", "password hashing failed")
		return
	}

	// Atomic consume: the guarded UPDATE re-checks token + status so a raced
	// double-accept (or an invite consumed since the lookup) affects 0 rows.
	res, err := g.db.ExecContext(r.Context(), `
		UPDATE np_saas_team_members
		SET status = 'active', joined_at = NOW(), password_hash = $1,
		    name = COALESCE(NULLIF($2, ''), name), invite_token_hash = NULL
		WHERE id = $3 AND invite_token_hash = $4 AND status = 'invited'`,
		hash, req.Name, inv.MemberID, sha256Hex(req.Token))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "invite accept failed")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeInviteNotFound(w)
		return
	}

	// Login JWT scoped to the INVITING tenant (from the invite row — never
	// client input), tagged with the member identity + role.
	name := firstNonEmpty(req.Name, inv.Name, inv.Email)
	claims := sessionClaims(inv.TenantID, inv.Email, name, saas.Tier(inv.Tier), time.Now().UTC())
	claims["role"] = inv.Role
	claims["member_id"] = inv.MemberID
	token, err := saas.SignHS256(claims, g.cfg.JWTSecret)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "token_error", "session token signing failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token":      token,
		"tenant_id":  inv.TenantID,
		"email":      inv.Email,
		"name":       name,
		"role":       inv.Role,
		"tier":       inv.Tier,
		"expires_in": int64(sessionTTL.Seconds()),
	})
}
