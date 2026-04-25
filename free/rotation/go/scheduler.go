package main

// scheduler.go — core rotation logic
//
// processDueSchedules queries np_secret_rotation_schedules for entries where
// next_rotation_at <= now() and enabled = TRUE. For each due secret:
//   1. Generate new value (deterministic by key-name pattern)
//   2. Call the rotation hook (default: write to NSELF_SECRETS_PATH/.env.secrets)
//   3. Trigger nself reload (SIGHUP to running stack via Unix domain socket or
//      by writing a reload sentinel file that the CLI watchdog detects)
//   4. Run verify (GET /health on the configured health endpoint)
//   5. If verify fails: roll back (restore old value), alert operator
//   6. Record result in np_secret_rotation_events
//   7. Update next_rotation_at = now() + interval_days

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"strings"
	"time"
)

// rotationService holds dependencies for the rotation service.
type rotationService struct {
	db      *sql.DB
	dryRun  bool
	enabled bool
}

// RotationScheduleRow mirrors np_secret_rotation_schedules.
type RotationScheduleRow struct {
	ID               string
	SourceAccountID  string
	SecretName       string
	IntervalDays     int
	WindowDays       int
	NotifyEmail      sql.NullString
	NotifyWebhook    sql.NullString
	LastRotatedAt    sql.NullTime
	NextRotationAt   time.Time
}

// RotationEventRow mirrors np_secret_rotation_events.
type RotationEventRow struct {
	ID              string
	SourceAccountID string
	SecretName      string
	RotatedAt       time.Time
	Status          string
	VerifyResult    sql.NullString
	ErrorDetail     sql.NullString
}

// processDueSchedules finds and rotates all secrets due for rotation.
// Returns counts of rotated, skipped, and error messages.
func (s *rotationService) processDueSchedules(ctx context.Context) (rotated, skipped int, errs []string) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, source_account_id, secret_name, interval_days, window_days,
		       notify_email, notify_webhook, last_rotated_at, next_rotation_at
		FROM np_secret_rotation_schedules
		WHERE enabled = TRUE AND next_rotation_at <= now()
		ORDER BY next_rotation_at ASC
	`)
	if err != nil {
		errs = append(errs, fmt.Sprintf("query schedules: %v", err))
		return
	}
	defer rows.Close()

	var due []RotationScheduleRow
	for rows.Next() {
		var r RotationScheduleRow
		if err := rows.Scan(
			&r.ID, &r.SourceAccountID, &r.SecretName, &r.IntervalDays, &r.WindowDays,
			&r.NotifyEmail, &r.NotifyWebhook, &r.LastRotatedAt, &r.NextRotationAt,
		); err != nil {
			errs = append(errs, fmt.Sprintf("scan: %v", err))
			continue
		}
		due = append(due, r)
	}
	if err := rows.Err(); err != nil {
		errs = append(errs, fmt.Sprintf("rows: %v", err))
	}

	for _, r := range due {
		if s.dryRun {
			log.Printf("rotation: DRY-RUN — would rotate %s", r.SecretName)
			skipped++
			continue
		}
		if err := s.rotateOne(ctx, r); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", r.SecretName, err))
		} else {
			rotated++
		}
	}
	return
}

// rotateOne performs the full rotation cycle for a single secret.
func (s *rotationService) rotateOne(ctx context.Context, r RotationScheduleRow) error {
	newValue, requiresManual := generateValue(r.SecretName)
	if requiresManual {
		log.Printf("rotation: %s requires manual rotation — skipping", r.SecretName)
		return s.recordEvent(ctx, r.SourceAccountID, r.SecretName, "failed",
			"requires manual rotation through provider", "manual-only secret")
	}

	// Write new value to secrets file.
	secretsPath := secretsFilePath()
	oldValue, _ := readSecretFromFile(secretsPath, r.SecretName)

	if err := writeSecretToFile(secretsPath, r.SecretName, newValue); err != nil {
		return s.recordEvent(ctx, r.SourceAccountID, r.SecretName, "failed",
			"", fmt.Sprintf("write failed: %v", err))
	}

	// Trigger reload sentinel (CLI watchdog detects .nself/reload-signal).
	_ = writeReloadSentinel()

	// Verify health after rotation.
	healthURL := os.Getenv("NSELF_HEALTH_URL")
	if healthURL == "" {
		healthURL = "http://localhost:4000/health"
	}
	verifyOK, verifyMsg := verifyHealth(healthURL, 30*time.Second)

	if !verifyOK {
		// Roll back.
		if oldValue != "" {
			_ = writeSecretToFile(secretsPath, r.SecretName, oldValue)
			_ = writeReloadSentinel()
		}
		_ = notifyFailure(r, verifyMsg)
		return s.recordEvent(ctx, r.SourceAccountID, r.SecretName, "rolled_back",
			verifyMsg, "health check failed after rotation")
	}

	// Update schedule.
	next := time.Now().UTC().AddDate(0, 0, r.IntervalDays)
	_, err := s.db.ExecContext(ctx, `
		UPDATE np_secret_rotation_schedules
		SET last_rotated_at = now(), next_rotation_at = $1, updated_at = now()
		WHERE id = $2
	`, next, r.ID)
	if err != nil {
		log.Printf("rotation: updating schedule for %s: %v", r.SecretName, err)
	}

	_ = notifySuccess(r)
	return s.recordEvent(ctx, r.SourceAccountID, r.SecretName, "ok", verifyMsg, "")
}

// recordEvent inserts a row into np_secret_rotation_events.
func (s *rotationService) recordEvent(ctx context.Context, accountID, secretName, status, verifyResult, errorDetail string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO np_secret_rotation_events
		    (source_account_id, secret_name, rotated_at, status, verify_result, error_detail)
		VALUES ($1, $2, now(), $3, NULLIF($4,''), NULLIF($5,''))
	`, accountID, secretName, status, verifyResult, errorDetail)
	return err
}

// listSchedules returns all rotation schedules.
func (s *rotationService) listSchedules(ctx context.Context) ([]RotationScheduleRow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, source_account_id, secret_name, interval_days, window_days,
		       notify_email, notify_webhook, last_rotated_at, next_rotation_at
		FROM np_secret_rotation_schedules
		ORDER BY secret_name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []RotationScheduleRow
	for rows.Next() {
		var r RotationScheduleRow
		if err := rows.Scan(
			&r.ID, &r.SourceAccountID, &r.SecretName, &r.IntervalDays, &r.WindowDays,
			&r.NotifyEmail, &r.NotifyWebhook, &r.LastRotatedAt, &r.NextRotationAt,
		); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// listEvents returns the last N rotation events.
func (s *rotationService) listEvents(ctx context.Context, limit int) ([]RotationEventRow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, source_account_id, secret_name, rotated_at, status, verify_result, error_detail
		FROM np_secret_rotation_events
		ORDER BY rotated_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []RotationEventRow
	for rows.Next() {
		var r RotationEventRow
		if err := rows.Scan(
			&r.ID, &r.SourceAccountID, &r.SecretName, &r.RotatedAt, &r.Status,
			&r.VerifyResult, &r.ErrorDetail,
		); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// generateValue returns a new secret value based on the key name pattern.
// Returns ("", true) when the secret requires manual provider-side rotation.
func generateValue(key string) (value string, requiresManual bool) {
	upper := strings.ToUpper(key)
	switch {
	case strings.Contains(upper, "API_KEY") || strings.Contains(upper, "_TOKEN"):
		return "", true
	case strings.HasSuffix(upper, "_PASSWORD") || strings.HasSuffix(upper, "_PASS"):
		return randomString(32), false
	default:
		return randomString(64), false
	}
}

// randomString returns a cryptographically random alphanumeric string.
func randomString(length int) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		b[i] = chars[n.Int64()]
	}
	return string(b)
}

// secretsFilePath returns the path to the .env.secrets file.
func secretsFilePath() string {
	if p := os.Getenv("NSELF_SECRETS_PATH"); p != "" {
		return p
	}
	return ".env.secrets"
}

// readSecretFromFile reads a single KEY=VALUE from the secrets file.
func readSecretFromFile(path, key string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	prefix := key + "="
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimPrefix(line, prefix), nil
		}
	}
	return "", fmt.Errorf("key %q not found", key)
}

// writeSecretToFile updates or inserts KEY=VALUE in the secrets file (0600).
func writeSecretToFile(path, key, value string) error {
	var lines []string
	existing, _ := os.ReadFile(path)
	prefix := key + "="
	found := false
	for _, line := range strings.Split(string(existing), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			lines = append(lines, key+"="+value)
			found = true
		} else if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	if !found {
		lines = append(lines, key+"="+value)
	}
	content := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(path, []byte(content), 0600)
}

// writeReloadSentinel writes a sentinel file that the CLI watchdog monitors.
func writeReloadSentinel() error {
	sentinelDir := ".nself"
	if err := os.MkdirAll(sentinelDir, 0700); err != nil {
		return err
	}
	return os.WriteFile(sentinelDir+"/reload-signal", []byte(time.Now().UTC().Format(time.RFC3339)), 0600)
}

// verifyHealth polls the given URL until it returns 200 or the deadline passes.
func verifyHealth(url string, timeout time.Duration) (ok bool, msg string) {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 5 * time.Second}
	for time.Now().Before(deadline) {
		resp, err := client.Get(url) //nolint:noctx
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return true, "health check passed"
		}
		if err == nil {
			resp.Body.Close()
		}
		time.Sleep(2 * time.Second)
	}
	return false, fmt.Sprintf("health check at %s timed out after %v", url, timeout)
}

// notifyFailure sends an operator alert when rotation + rollback occurred.
func notifyFailure(r RotationScheduleRow, reason string) error {
	email := os.Getenv("NSELF_SECRET_ROTATION_NOTIFY_EMAIL")
	if email == "" && r.NotifyEmail.Valid {
		email = r.NotifyEmail.String
	}
	if email != "" {
		log.Printf("rotation: ALERT — %s rotation failed (%s). Notify: %s", r.SecretName, reason, email)
	}
	return nil
}

// notifySuccess sends an operator notice when rotation succeeded.
func notifySuccess(r RotationScheduleRow) error {
	email := os.Getenv("NSELF_SECRET_ROTATION_NOTIFY_EMAIL")
	if email == "" && r.NotifyEmail.Valid {
		email = r.NotifyEmail.String
	}
	if email != "" {
		log.Printf("rotation: SUCCESS — %s rotated. Notify: %s", r.SecretName, email)
	}
	return nil
}
