// Package mfa — recovery.go
//
// Recovery code management: list masked codes, regenerate, use.
// Delegates to TOTPService for the heavy lifting; this file adds the HTTP
// handler-facing types and the masked listing helper.
package mfa

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// MaskedCode is a recovery code with all but the last 4 characters redacted.
type MaskedCode struct {
	ID       string `json:"id"`
	Masked   string `json:"masked"`
	Used     bool   `json:"used"`
	UsedAt   *time.Time `json:"used_at,omitempty"`
}

// RecoveryService provides read-only and management operations over backup
// codes. The underlying write operations live on TOTPService to keep
// cryptographic concerns in one place.
type RecoveryService struct {
	db *pgxpool.Pool
}

// NewRecoveryService creates a RecoveryService.
func NewRecoveryService(db *pgxpool.Pool) *RecoveryService {
	return &RecoveryService{db: db}
}

// ListCodes returns masked representations of all recovery codes for userID.
// Plaintext codes are never returned by this method; only the mask is exposed
// to allow the user to identify which codes remain active.
func (s *RecoveryService) ListCodes(ctx context.Context, userID string) ([]MaskedCode, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	rows, err := s.db.Query(ctx,
		`SELECT id, code_hash, used_at IS NOT NULL, used_at
		   FROM np_mfa_recovery_codes
		  WHERE user_id = $1
		  ORDER BY created_at`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list codes: %w", err)
	}
	defer rows.Close()

	var result []MaskedCode
	for rows.Next() {
		var m MaskedCode
		var hash string
		if err := rows.Scan(&m.ID, &hash, &m.Used, &m.UsedAt); err != nil {
			return nil, err
		}
		// We can't reverse the bcrypt hash, so we just indicate "code available"
		// vs "code used" without revealing any portion of the actual code.
		if m.Used {
			m.Masked = strings.Repeat("*", 8)
		} else {
			m.Masked = "****-****"
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

// CountUnused returns the number of unused recovery codes for userID.
func (s *RecoveryService) CountUnused(ctx context.Context, userID string) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var count int
	err := s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM np_mfa_recovery_codes WHERE user_id = $1 AND used_at IS NULL`,
		userID,
	).Scan(&count)
	return count, err
}
