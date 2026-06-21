package internal

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
)

func GenerateSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "whsec_" + hex.EncodeToString(b), nil
}

// --- Endpoint CRUD -----------------------------------------------------------

// CreateEndpoint inserts a new webhook endpoint.
func CreateEndpoint(ctx context.Context, pool *pgxpool.Pool, url string, events []string, description *string, secret string, headersJSON string, metadataJSON string) (*Endpoint, error) {
	var e Endpoint
	err := pool.QueryRow(ctx, `
		INSERT INTO np_webhooks_endpoints (url, description, secret, events, headers, metadata)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6::jsonb)
		RETURNING id, url, description, secret, events, headers::text, enabled,
		          failure_count, last_success_at, last_failure_at, disabled_at,
		          disabled_reason, metadata::text, created_at, updated_at
	`, url, description, secret, events, headersJSON, metadataJSON).Scan(
		&e.ID, &e.URL, &e.Description, &e.Secret, &e.Events, &e.Headers,
		&e.Enabled, &e.FailureCount, &e.LastSuccessAt, &e.LastFailureAt,
		&e.DisabledAt, &e.DisabledReason, &e.Metadata, &e.CreatedAt, &e.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// GetEndpoint returns a single endpoint by ID.
func GetEndpoint(ctx context.Context, pool *pgxpool.Pool, id string) (*Endpoint, error) {
	var e Endpoint
	err := pool.QueryRow(ctx, `
		SELECT id, url, description, secret, events, headers::text, enabled,
		       failure_count, last_success_at, last_failure_at, disabled_at,
		       disabled_reason, metadata::text, created_at, updated_at
		FROM np_webhooks_endpoints WHERE id = $1
	`, id).Scan(
		&e.ID, &e.URL, &e.Description, &e.Secret, &e.Events, &e.Headers,
		&e.Enabled, &e.FailureCount, &e.LastSuccessAt, &e.LastFailureAt,
		&e.DisabledAt, &e.DisabledReason, &e.Metadata, &e.CreatedAt, &e.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// ListEndpoints returns all endpoints, optionally filtered by enabled status.
func ListEndpoints(ctx context.Context, pool *pgxpool.Pool, enabledFilter *bool) ([]Endpoint, error) {
	query := `SELECT id, url, description, secret, events, headers::text, enabled,
	                 failure_count, last_success_at, last_failure_at, disabled_at,
	                 disabled_reason, metadata::text, created_at, updated_at
	          FROM np_webhooks_endpoints WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if enabledFilter != nil {
		query += fmt.Sprintf(" AND enabled = $%d", argIdx)
		args = append(args, *enabledFilter)
		argIdx++
	}

	query += " ORDER BY created_at DESC"

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Endpoint
	for rows.Next() {
		var e Endpoint
		if err := rows.Scan(
			&e.ID, &e.URL, &e.Description, &e.Secret, &e.Events, &e.Headers,
			&e.Enabled, &e.FailureCount, &e.LastSuccessAt, &e.LastFailureAt,
			&e.DisabledAt, &e.DisabledReason, &e.Metadata, &e.CreatedAt, &e.UpdatedAt,
		); err != nil {
			return nil, err
		}
		results = append(results, e)
	}
	return results, rows.Err()
}

// UpdateEndpoint updates fields on an existing endpoint. Only non-nil fields
// are changed. Returns the updated endpoint or nil if not found.
func UpdateEndpoint(ctx context.Context, pool *pgxpool.Pool, id string, url *string, description *string, events []string, headersJSON *string, enabled *bool, metadataJSON *string) (*Endpoint, error) {
	updates := []string{"updated_at = NOW()"}
	args := []interface{}{}
	argIdx := 1

	if url != nil {
		updates = append(updates, fmt.Sprintf("url = $%d", argIdx))
		args = append(args, *url)
		argIdx++
	}
	if description != nil {
		updates = append(updates, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, *description)
		argIdx++
	}
	if events != nil {
		updates = append(updates, fmt.Sprintf("events = $%d", argIdx))
		args = append(args, events)
		argIdx++
	}
	if headersJSON != nil {
		updates = append(updates, fmt.Sprintf("headers = $%d::jsonb", argIdx))
		args = append(args, *headersJSON)
		argIdx++
	}
	if enabled != nil {
		updates = append(updates, fmt.Sprintf("enabled = $%d", argIdx))
		args = append(args, *enabled)
		argIdx++
	}
	if metadataJSON != nil {
		updates = append(updates, fmt.Sprintf("metadata = $%d::jsonb", argIdx))
		args = append(args, *metadataJSON)
		argIdx++
	}

	if len(updates) == 1 {
		return GetEndpoint(ctx, pool, id)
	}

	args = append(args, id)

	query := fmt.Sprintf(`UPDATE np_webhooks_endpoints SET %s WHERE id = $%d
		RETURNING id, url, description, secret, events, headers::text, enabled,
		          failure_count, last_success_at, last_failure_at, disabled_at,
		          disabled_reason, metadata::text, created_at, updated_at`,
		joinStrings(updates, ", "), argIdx)

	var e Endpoint
	err := pool.QueryRow(ctx, query, args...).Scan(
		&e.ID, &e.URL, &e.Description, &e.Secret, &e.Events, &e.Headers,
		&e.Enabled, &e.FailureCount, &e.LastSuccessAt, &e.LastFailureAt,
		&e.DisabledAt, &e.DisabledReason, &e.Metadata, &e.CreatedAt, &e.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// DeleteEndpoint removes an endpoint by ID. Returns true if a row was deleted.
func DeleteEndpoint(ctx context.Context, pool *pgxpool.Pool, id string) (bool, error) {
	tag, err := pool.Exec(ctx,
		"DELETE FROM np_webhooks_endpoints WHERE id = $1", id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// --- Endpoint status tracking ------------------------------------------------

// RecordEndpointSuccess resets failure count and updates last_success_at.
