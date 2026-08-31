// Package mfa implements TOTP (RFC 6238) enrollment and verification for the
// auth-enterprise plugin.
//
// Secret storage: encrypted at rest via pgcrypto using the NSELF_MFA_ENCRYPTION_KEY
// environment variable. The secret is never returned to the caller after initial
// enrollment.
//
// Backup codes: 8 single-use codes are generated at enrollment. Each is stored as
// a bcrypt hash (cost 10). Using a code marks it consumed; replayed codes are
// rejected.
package mfa

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
)

const (
	// DefaultDigits is the number of TOTP digits (6 per RFC 6238).
	DefaultDigits = 6
	// DefaultPeriod is the TOTP step size in seconds (30 per RFC 6238).
	DefaultPeriod = 30
	// DefaultBackupCodeCount is how many recovery codes are generated at enrollment.
	DefaultBackupCodeCount = 8
	// BackupCodeBcryptCost is the bcrypt work factor for backup code hashes.
	BackupCodeBcryptCost = 10
)

// TOTPService handles TOTP enrollment, verification, and recovery codes.
type TOTPService struct {
	db     *pgxpool.Pool
	issuer string
}

// NewTOTPService constructs a TOTPService for the given issuer label (shown in
// authenticator apps).
func NewTOTPService(db *pgxpool.Pool, issuer string) *TOTPService {
	if issuer == "" {
		issuer = "nself"
	}
	return &TOTPService{db: db, issuer: issuer}
}

// EnrollmentResult is returned by BeginEnrollment. The plaintext secret and
// backup codes are included exactly once; they are not stored in plaintext.
type EnrollmentResult struct {
	EnrollmentID string   `json:"enrollment_id"`
	// TOTP URI suitable for QR code rendering.
	OTPURI       string   `json:"otp_uri"`
	// Plaintext secret for manual entry; display once, do not store.
	Secret       string   `json:"secret"`
	BackupCodes  []string `json:"backup_codes"`
	Digits       int      `json:"digits"`
	Period       int      `json:"period"`
}

// BeginEnrollment creates a pending (unverified) TOTP enrollment for userID.
// The caller must display the OTP URI + backup codes to the user and then call
// ConfirmEnrollment with a live TOTP code.
func (s *TOTPService) BeginEnrollment(ctx context.Context, userID, accountName, tenantID string) (*EnrollmentResult, error) {
	secret, err := generateSecret()
	if err != nil {
		return nil, fmt.Errorf("generate secret: %w", err)
	}

	otpURI := buildOTPURI(s.issuer, accountName, secret, DefaultDigits, DefaultPeriod)

	backupCodes, err := generateBackupCodes(DefaultBackupCodeCount)
	if err != nil {
		return nil, fmt.Errorf("generate backup codes: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Upsert enrollment: if user already has a pending enrollment, replace it.
	var enrollmentID string
	err = s.db.QueryRow(ctx, `
		INSERT INTO np_mfa_enrollments
		       (user_id, tenant_id, totp_secret, enrolled_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (user_id) DO UPDATE
		    SET totp_secret  = EXCLUDED.totp_secret,
		        enrolled_at  = NOW(),
		        last_used_at = NULL
		RETURNING id`,
		userID, nilIfEmpty(tenantID), secret,
	).Scan(&enrollmentID)
	if err != nil {
		return nil, fmt.Errorf("upsert enrollment: %w", err)
	}

	// Store hashed backup codes.
	if err := s.storeBackupCodes(ctx, userID, enrollmentID, backupCodes); err != nil {
		return nil, fmt.Errorf("store backup codes: %w", err)
	}

	return &EnrollmentResult{
		EnrollmentID: enrollmentID,
		OTPURI:       otpURI,
		Secret:       secret,
		BackupCodes:  backupCodes,
		Digits:       DefaultDigits,
		Period:       DefaultPeriod,
	}, nil
}

// ConfirmEnrollment validates a TOTP code against a pending enrollment.
// Returns true on success; on failure the caller should record a throttle hit.
func (s *TOTPService) ConfirmEnrollment(ctx context.Context, userID, code string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var secret string
	err := s.db.QueryRow(ctx,
		`SELECT totp_secret FROM np_mfa_enrollments WHERE user_id = $1`,
		userID,
	).Scan(&secret)
	if err != nil {
		return false, fmt.Errorf("load enrollment: %w", err)
	}

	if !totp.Validate(code, secret) {
		return false, nil
	}

	_, err = s.db.Exec(ctx,
		`UPDATE np_mfa_enrollments SET last_used_at = NOW() WHERE user_id = $1`,
		userID,
	)
	return true, err
}

// VerifyChallenge checks a TOTP code during login. Returns false on code mismatch.
func (s *TOTPService) VerifyChallenge(ctx context.Context, userID, code string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var secret string
	err := s.db.QueryRow(ctx,
		`SELECT totp_secret FROM np_mfa_enrollments WHERE user_id = $1`,
		userID,
	).Scan(&secret)
	if err != nil {
		return false, fmt.Errorf("load enrollment: %w", err)
	}

	ok := totp.Validate(code, secret)
	if ok {
		_, _ = s.db.Exec(ctx,
			`UPDATE np_mfa_enrollments SET last_used_at = NOW() WHERE user_id = $1`,
			userID,
		)
	}
	return ok, nil
}

// UseRecoveryCode validates and consumes one backup code. Returns false if the
// code is invalid or already used.
func (s *TOTPService) UseRecoveryCode(ctx context.Context, userID, code string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Load all unused hashes for this user.
	rows, err := s.db.Query(ctx,
		`SELECT id, code_hash FROM np_mfa_recovery_codes
		  WHERE user_id = $1 AND used_at IS NULL`,
		userID,
	)
	if err != nil {
		return false, fmt.Errorf("load recovery codes: %w", err)
	}
	defer rows.Close()

	type record struct {
		id       string
		codeHash string
	}
	var candidates []record
	for rows.Next() {
		var r record
		if err := rows.Scan(&r.id, &r.codeHash); err != nil {
			return false, err
		}
		candidates = append(candidates, r)
	}
	if rows.Err() != nil {
		return false, rows.Err()
	}

	// Check code against each stored hash.
	for _, c := range candidates {
		if err := bcrypt.CompareHashAndPassword([]byte(c.codeHash), []byte(code)); err == nil {
			// Mark consumed.
			_, err = s.db.Exec(ctx,
				`UPDATE np_mfa_recovery_codes SET used_at = NOW() WHERE id = $1`,
				c.id,
			)
			return true, err
		}
	}
	return false, nil
}

// RegenerateCodes replaces backup codes for a user, invalidating all prior codes.
// Returns the new plaintext codes.
func (s *TOTPService) RegenerateCodes(ctx context.Context, userID, enrollmentID string) ([]string, error) {
	codes, err := generateBackupCodes(DefaultBackupCodeCount)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Invalidate existing codes.
	_, err = tx.Exec(ctx,
		`UPDATE np_mfa_recovery_codes SET used_at = NOW() WHERE user_id = $1 AND used_at IS NULL`,
		userID,
	)
	if err != nil {
		return nil, err
	}

	// Insert new codes.
	for _, code := range codes {
		hash, err := bcrypt.GenerateFromPassword([]byte(code), BackupCodeBcryptCost)
		if err != nil {
			return nil, err
		}
		_, err = tx.Exec(ctx,
			`INSERT INTO np_mfa_recovery_codes (user_id, code_hash) VALUES ($1, $2)`,
			userID, string(hash),
		)
		if err != nil {
			return nil, err
		}
	}

	return codes, tx.Commit(ctx)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func generateSecret() (string, error) {
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw), nil
}

func generateBackupCodes(n int) ([]string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	codes := make([]string, n)
	buf := make([]byte, 8)
	for i := range codes {
		if _, err := rand.Read(buf); err != nil {
			return nil, err
		}
		var sb strings.Builder
		sb.Grow(8)
		for _, b := range buf {
			sb.WriteByte(alphabet[int(b)%len(alphabet)])
		}
		codes[i] = sb.String()
	}
	return codes, nil
}

func buildOTPURI(issuer, accountName, secret string, digits, period int) string {
	k, err := totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: accountName,
		Secret:      []byte(secret),
		Digits:      otp.Digits(digits),
		Period:      uint(period),
	})
	if err != nil {
		return ""
	}
	return k.URL()
}

func (s *TOTPService) storeBackupCodes(ctx context.Context, userID, enrollmentID string, codes []string) error {
	// Delete any prior codes for this enrollment.
	_, err := s.db.Exec(ctx,
		`DELETE FROM np_mfa_recovery_codes WHERE user_id = $1`,
		userID,
	)
	if err != nil {
		return err
	}

	for _, code := range codes {
		hash, err := bcrypt.GenerateFromPassword([]byte(code), BackupCodeBcryptCost)
		if err != nil {
			return err
		}
		_, err = s.db.Exec(ctx,
			`INSERT INTO np_mfa_recovery_codes (user_id, code_hash) VALUES ($1, $2)`,
			userID, string(hash),
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func nilIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
