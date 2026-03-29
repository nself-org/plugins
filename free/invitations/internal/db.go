package internal

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Invitation represents a row in np_invitations_invitations.
type Invitation struct {
	ID         string     `json:"id"`
	Email      string     `json:"email"`
	Role       string     `json:"role"`
	Token      string     `json:"token"`
	Status     string     `json:"status"`
	InvitedBy  string     `json:"invited_by"`
	ExpiresAt  *time.Time `json:"expires_at"`
	AcceptedAt *time.Time `json:"accepted_at"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// Migrate creates the required tables if they do not exist.
func Migrate(pool *pgxpool.Pool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS np_invitations_invitations (
			id          TEXT PRIMARY KEY,
			email       TEXT NOT NULL,
			role        TEXT NOT NULL DEFAULT 'member',
			token       TEXT NOT NULL UNIQUE,
			status      TEXT NOT NULL DEFAULT 'pending',
			invited_by  TEXT NOT NULL,
			expires_at  TIMESTAMPTZ,
			accepted_at TIMESTAMPTZ,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_np_invitations_status
			ON np_invitations_invitations (status);

		CREATE INDEX IF NOT EXISTS idx_np_invitations_email
			ON np_invitations_invitations (email);

		CREATE INDEX IF NOT EXISTS idx_np_invitations_token
			ON np_invitations_invitations (token);
	`)
	return err
}

// InsertInvitation creates a new invitation record.
func InsertInvitation(ctx context.Context, pool *pgxpool.Pool, inv *Invitation) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO np_invitations_invitations (id, email, role, token, status, invited_by, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, inv.ID, inv.Email, inv.Role, inv.Token, inv.Status, inv.InvitedBy, inv.ExpiresAt)
	return err
}

// GetInvitation returns a single invitation by ID.
func GetInvitation(ctx context.Context, pool *pgxpool.Pool, id string) (*Invitation, error) {
	var inv Invitation
	err := pool.QueryRow(ctx, `
		SELECT id, email, role, token, status, invited_by, expires_at, accepted_at, created_at, updated_at
		FROM np_invitations_invitations WHERE id = $1
	`, id).Scan(&inv.ID, &inv.Email, &inv.Role, &inv.Token, &inv.Status, &inv.InvitedBy,
		&inv.ExpiresAt, &inv.AcceptedAt, &inv.CreatedAt, &inv.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

// GetInvitationByToken returns a single invitation by its unique token.
func GetInvitationByToken(ctx context.Context, pool *pgxpool.Pool, token string) (*Invitation, error) {
	var inv Invitation
	err := pool.QueryRow(ctx, `
		SELECT id, email, role, token, status, invited_by, expires_at, accepted_at, created_at, updated_at
		FROM np_invitations_invitations WHERE token = $1
	`, token).Scan(&inv.ID, &inv.Email, &inv.Role, &inv.Token, &inv.Status, &inv.InvitedBy,
		&inv.ExpiresAt, &inv.AcceptedAt, &inv.CreatedAt, &inv.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

// ListInvitations returns invitations with optional status filtering and pagination.
func ListInvitations(ctx context.Context, pool *pgxpool.Pool, status string, limit, offset int) ([]Invitation, error) {
	query := `SELECT id, email, role, token, status, invited_by, expires_at, accepted_at, created_at, updated_at
		FROM np_invitations_invitations WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if status != "" {
		query += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, status)
		argIdx++
	}

	query += " ORDER BY created_at DESC"
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Invitation
	for rows.Next() {
		var inv Invitation
		if err := rows.Scan(&inv.ID, &inv.Email, &inv.Role, &inv.Token, &inv.Status, &inv.InvitedBy,
			&inv.ExpiresAt, &inv.AcceptedAt, &inv.CreatedAt, &inv.UpdatedAt); err != nil {
			return nil, err
		}
		results = append(results, inv)
	}
	return results, rows.Err()
}

// RevokeInvitation sets the invitation status to "revoked".
func RevokeInvitation(ctx context.Context, pool *pgxpool.Pool, id string) error {
	tag, err := pool.Exec(ctx, `
		UPDATE np_invitations_invitations
		SET status = 'revoked', updated_at = NOW()
		WHERE id = $1 AND status = 'pending'
	`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("invitation not found or not in pending status")
	}
	return nil
}

// AcceptInvitation marks the invitation as accepted.
func AcceptInvitation(ctx context.Context, pool *pgxpool.Pool, token string) (*Invitation, error) {
	var inv Invitation
	err := pool.QueryRow(ctx, `
		UPDATE np_invitations_invitations
		SET status = 'accepted', accepted_at = NOW(), updated_at = NOW()
		WHERE token = $1 AND status = 'pending'
		RETURNING id, email, role, token, status, invited_by, expires_at, accepted_at, created_at, updated_at
	`, token).Scan(&inv.ID, &inv.Email, &inv.Role, &inv.Token, &inv.Status, &inv.InvitedBy,
		&inv.ExpiresAt, &inv.AcceptedAt, &inv.CreatedAt, &inv.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

// UpdateInvitationStatus updates the status and updated_at timestamp for an invitation.
func UpdateInvitationStatus(ctx context.Context, pool *pgxpool.Pool, id, status string) error {
	tag, err := pool.Exec(ctx, `
		UPDATE np_invitations_invitations
		SET status = $2, updated_at = NOW()
		WHERE id = $1
	`, id, status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("invitation not found")
	}
	return nil
}
