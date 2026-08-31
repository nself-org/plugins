// Package store manages CRUD for edge functions in the database.
package store

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const maxSourceBytes = 5 * 1024 * 1024 // 5 MB

var validName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

// ErrInvalidFunctionName is returned when the function name fails validation.
var ErrInvalidFunctionName = errors.New("invalid function name: use lowercase letters, digits, hyphens; start with a letter; max 63 chars")

// ErrSourceTooLarge is returned when the source exceeds the 5 MB limit.
var ErrSourceTooLarge = errors.New("source code exceeds 5 MB limit")

// ErrFunctionNotFound is returned when the function does not exist.
var ErrFunctionNotFound = errors.New("function not found")

// Function represents a deployed edge function.
type Function struct {
	ID               uuid.UUID
	Name             string
	Description      string
	Status           string
	InvokeCount      int64
	ErrorCount       int64
	SourceAccountID  string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Version represents a deployed version of a function.
type Version struct {
	ID         uuid.UUID
	FunctionID uuid.UUID
	Version    int
	SourceCode string
	IsActive   bool
	DeployedAt time.Time
	DeployedBy string
}

// EnvVar is a per-function env variable.
type EnvVar struct {
	FunctionID uuid.UUID
	Key        string
	Value      string
}

// Store wraps the database pool for function persistence.
type Store struct {
	pool *pgxpool.Pool
}

// New creates a new Store.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// ValidateName checks the function name against the allowed pattern.
func ValidateName(name string) error {
	if !validName.MatchString(name) {
		return ErrInvalidFunctionName
	}
	return nil
}

// ValidateSource checks source code size.
func ValidateSource(src string) error {
	if len(src) > maxSourceBytes {
		return ErrSourceTooLarge
	}
	return nil
}

// CreateOrUpdate upserts a function record and inserts a new version.
// Returns the new version number.
func (s *Store) CreateOrUpdate(ctx context.Context, name, description, source, deployedBy string) (int, error) {
	if err := ValidateName(name); err != nil {
		return 0, err
	}
	if err := ValidateSource(source); err != nil {
		return 0, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Upsert function record.
	var fnID uuid.UUID
	row := tx.QueryRow(ctx, `
		INSERT INTO np_edge_functions (name, description, updated_at)
		VALUES ($1, $2, now())
		ON CONFLICT (name) DO UPDATE
		  SET description = EXCLUDED.description, updated_at = now()
		RETURNING id
	`, name, description)
	if err := row.Scan(&fnID); err != nil {
		return 0, fmt.Errorf("upsert function: %w", err)
	}

	// Deactivate all existing versions.
	if _, err := tx.Exec(ctx, `
		UPDATE np_edge_function_versions SET is_active = false WHERE function_id = $1
	`, fnID); err != nil {
		return 0, fmt.Errorf("deactivate versions: %w", err)
	}

	// Get the next version number.
	var maxVer int
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(version), 0) FROM np_edge_function_versions WHERE function_id = $1
	`, fnID).Scan(&maxVer); err != nil {
		return 0, fmt.Errorf("get max version: %w", err)
	}
	newVer := maxVer + 1

	// Insert new active version.
	if _, err := tx.Exec(ctx, `
		INSERT INTO np_edge_function_versions (function_id, version, source_code, is_active, deployed_by)
		VALUES ($1, $2, $3, true, $4)
	`, fnID, newVer, source, deployedBy); err != nil {
		return 0, fmt.Errorf("insert version: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return newVer, nil
}

// GetActive returns the active source code for a function.
func (s *Store) GetActive(ctx context.Context, name string) (uuid.UUID, string, error) {
	var fnID uuid.UUID
	var source string
	err := s.pool.QueryRow(ctx, `
		SELECT f.id, v.source_code
		FROM np_edge_functions f
		JOIN np_edge_function_versions v ON v.function_id = f.id AND v.is_active = true
		WHERE f.name = $1 AND f.status = 'active'
	`, name).Scan(&fnID, &source)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, "", ErrFunctionNotFound
	}
	return fnID, source, err
}

// List returns all functions.
func (s *Store) List(ctx context.Context) ([]Function, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, COALESCE(description,''), status, invoke_count, error_count, source_account_id, created_at, updated_at
		FROM np_edge_functions ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var fns []Function
	for rows.Next() {
		var f Function
		if err := rows.Scan(&f.ID, &f.Name, &f.Description, &f.Status,
			&f.InvokeCount, &f.ErrorCount, &f.SourceAccountID, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, err
		}
		fns = append(fns, f)
	}
	return fns, rows.Err()
}

// GetByName returns a single function.
func (s *Store) GetByName(ctx context.Context, name string) (*Function, error) {
	var f Function
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, COALESCE(description,''), status, invoke_count, error_count, source_account_id, created_at, updated_at
		FROM np_edge_functions WHERE name = $1
	`, name).Scan(&f.ID, &f.Name, &f.Description, &f.Status,
		&f.InvokeCount, &f.ErrorCount, &f.SourceAccountID, &f.CreatedAt, &f.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrFunctionNotFound
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

// ListVersions returns all versions for a function ordered by version DESC.
func (s *Store) ListVersions(ctx context.Context, name string) ([]Version, error) {
	fn, err := s.GetByName(ctx, name)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, function_id, version, source_code, is_active, deployed_at, COALESCE(deployed_by,'')
		FROM np_edge_function_versions WHERE function_id = $1 ORDER BY version DESC
	`, fn.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var vers []Version
	for rows.Next() {
		var v Version
		if err := rows.Scan(&v.ID, &v.FunctionID, &v.Version, &v.SourceCode,
			&v.IsActive, &v.DeployedAt, &v.DeployedBy); err != nil {
			return nil, err
		}
		vers = append(vers, v)
	}
	return vers, rows.Err()
}

// ActivateVersion sets a specific version as active and deactivates others.
func (s *Store) ActivateVersion(ctx context.Context, name string, version int) error {
	fn, err := s.GetByName(ctx, name)
	if err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Verify the version exists.
	var exists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM np_edge_function_versions WHERE function_id=$1 AND version=$2)
	`, fn.ID, version).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("version %d not found for function %q", version, name)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE np_edge_function_versions SET is_active = false WHERE function_id = $1
	`, fn.ID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE np_edge_function_versions SET is_active = true WHERE function_id = $1 AND version = $2
	`, fn.ID, version); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Delete removes a function and all its versions (CASCADE).
func (s *Store) Delete(ctx context.Context, name string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM np_edge_functions WHERE name = $1`, name)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrFunctionNotFound
	}
	return nil
}

// IncrementInvokeCount increments the invoke counter (non-blocking fire-and-forget).
func (s *Store) IncrementInvokeCount(ctx context.Context, fnID uuid.UUID, isError bool) {
	col := "invoke_count"
	if isError {
		col = "error_count"
	}
	s.pool.Exec(ctx, fmt.Sprintf( //nolint:errcheck
		`UPDATE np_edge_functions SET %s = %s + 1, updated_at = now() WHERE id = $1`, col, col,
	), fnID)
}

// GetEnvVars returns the env vars for a function.
func (s *Store) GetEnvVars(ctx context.Context, name string) ([]EnvVar, error) {
	fn, err := s.GetByName(ctx, name)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT function_id, key, value FROM np_edge_function_env WHERE function_id = $1
	`, fn.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var envs []EnvVar
	for rows.Next() {
		var e EnvVar
		if err := rows.Scan(&e.FunctionID, &e.Key, &e.Value); err != nil {
			return nil, err
		}
		envs = append(envs, e)
	}
	return envs, rows.Err()
}

// SetEnvVar upserts an env variable for a function.
func (s *Store) SetEnvVar(ctx context.Context, name, key, value string) error {
	fn, err := s.GetByName(ctx, name)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO np_edge_function_env (function_id, key, value)
		VALUES ($1, $2, $3)
		ON CONFLICT (function_id, key) DO UPDATE SET value = EXCLUDED.value
	`, fn.ID, key, value)
	return err
}

// DeleteEnvVar removes an env variable for a function.
func (s *Store) DeleteEnvVar(ctx context.Context, name, key string) error {
	fn, err := s.GetByName(ctx, name)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `DELETE FROM np_edge_function_env WHERE function_id=$1 AND key=$2`, fn.ID, key)
	return err
}
