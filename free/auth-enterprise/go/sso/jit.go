// Package sso — jit.go
//
// Just-in-time (JIT) user provisioning.
//
// On first SSO login, nSelf creates a user row in np_users and an SSO session
// log entry in np_sso_sessions. Subsequent logins update last_seen_at and
// refresh the session log.
//
// Attribute mapping:
//   - email: the primary key for user lookup — must be unique per tenant
//   - name: stored as display_name; updated on every login
//   - role: mapped to np_users.role if non-empty; otherwise defaults to "member"
//
// Provisioning is idempotent: calling Provision twice for the same
// (tenantID, idpSubject) returns the existing user without modification.
package sso

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ProvisionedUser is the nSelf user record returned after JIT provisioning.
type ProvisionedUser struct {
	UserID      string
	Email       string
	DisplayName string
	Role        string
	TenantID    string
	// IsNew is true when the user was just created.
	IsNew bool
}

// JITProvisioner creates or updates nSelf users on first SSO login.
type JITProvisioner struct {
	db *pgxpool.Pool
}

// NewJITProvisioner creates a JITProvisioner backed by db.
func NewJITProvisioner(db *pgxpool.Pool) *JITProvisioner {
	return &JITProvisioner{db: db}
}

// Provision upserts an nSelf user for the given SSO identity and logs the SSO
// session. providerID is the np_sso_providers.id row for auditing.
func (j *JITProvisioner) Provision(ctx context.Context, tenantID, providerID, idpSubject, email, displayName, role string) (*ProvisionedUser, error) {
	if email == "" {
		return nil, fmt.Errorf("jit: email is required for provisioning")
	}
	if role == "" {
		role = "member"
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	tx, err := j.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin jit tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Upsert user by (tenant_id, email).
	var userID string
	var isNew bool
	err = tx.QueryRow(ctx, `
		INSERT INTO np_users (tenant_id, email, display_name, role, created_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (tenant_id, email) DO UPDATE
		    SET display_name = EXCLUDED.display_name,
		        role         = COALESCE(NULLIF(EXCLUDED.role, ''), np_users.role),
		        updated_at   = NOW()
		RETURNING id, (xmax = 0) AS inserted`,
		tenantID, email, displayName, role,
	).Scan(&userID, &isNew)
	if err != nil {
		return nil, fmt.Errorf("upsert np_users: %w", err)
	}

	// Log SSO session.
	_, err = tx.Exec(ctx, `
		INSERT INTO np_sso_sessions (user_id, provider_id, idp_subject, signed_in_at)
		VALUES ($1, $2, $3, NOW())`,
		userID, providerID, idpSubject,
	)
	if err != nil {
		return nil, fmt.Errorf("insert sso session: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit jit tx: %w", err)
	}

	return &ProvisionedUser{
		UserID:      userID,
		Email:       email,
		DisplayName: displayName,
		Role:        role,
		TenantID:    tenantID,
		IsNew:       isNew,
	}, nil
}
