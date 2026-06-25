package internal

import (
	pgx "github.com/jackc/pgx/v5"
	"context"
	"encoding/json"
	"fmt"
	"time"
)

func (d *DB) CreateService(req CreateServiceRequest) (*ServiceRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	serviceType := "_ntv._tcp"
	if req.ServiceType != "" {
		serviceType = req.ServiceType
	}
	host := "localhost"
	if req.Host != "" {
		host = req.Host
	}
	domain := "local"
	if req.Domain != "" {
		domain = req.Domain
	}
	txtRecords := json.RawMessage(`{}`)
	if req.TxtRecords != nil {
		txtRecords = *req.TxtRecords
	}
	metadata := json.RawMessage(`{}`)
	if req.Metadata != nil {
		metadata = *req.Metadata
	}

	var s ServiceRecord
	err := d.pool.QueryRow(ctx,
		`INSERT INTO np_mdns_services
			(source_account_id, service_name, service_type, port, host, domain, txt_records, metadata)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id, source_account_id, service_name, service_type, port, host, domain,
			txt_records, is_advertised, is_active, last_seen_at, metadata, created_at, updated_at`,
		d.sourceAccountID, req.ServiceName, serviceType, req.Port, host, domain, txtRecords, metadata,
	).Scan(&s.ID, &s.SourceAccountID, &s.ServiceName, &s.ServiceType, &s.Port, &s.Host, &s.Domain,
		&s.TxtRecords, &s.IsAdvertised, &s.IsActive, &s.LastSeenAt, &s.Metadata, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create service: %w", err)
	}
	return &s, nil
}

// GetService returns a single service by ID.
func (d *DB) GetService(id string) (*ServiceRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var s ServiceRecord
	err := d.pool.QueryRow(ctx,
		`SELECT id, source_account_id, service_name, service_type, port, host, domain,
			txt_records, is_advertised, is_active, last_seen_at, metadata, created_at, updated_at
		 FROM np_mdns_services WHERE id = $1 AND source_account_id = $2`, id, d.sourceAccountID,
	).Scan(&s.ID, &s.SourceAccountID, &s.ServiceName, &s.ServiceType, &s.Port, &s.Host, &s.Domain,
		&s.TxtRecords, &s.IsAdvertised, &s.IsActive, &s.LastSeenAt, &s.Metadata, &s.CreatedAt, &s.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get service: %w", err)
	}
	return &s, nil
}

// ListServices returns services with optional filtering and pagination.
// Size-cap exception: single DB operation — 63L scan loop with struct mapping; splitting would fragment a single SQL query across files.
func (d *DB) ListServices(serviceType string, isAdvertised, isActive *bool, limit, offset int) ([]ServiceRecord, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `SELECT id, source_account_id, service_name, service_type, port, host, domain,
		txt_records, is_advertised, is_active, last_seen_at, metadata, created_at, updated_at
		FROM np_mdns_services WHERE source_account_id = $1`
	countQuery := `SELECT COUNT(*) FROM np_mdns_services WHERE source_account_id = $1`
	args := []interface{}{d.sourceAccountID}
	countArgs := []interface{}{d.sourceAccountID}
	idx := 2

	if serviceType != "" {
		clause := fmt.Sprintf(" AND service_type = $%d", idx)
		query += clause
		countQuery += clause
		args = append(args, serviceType)
		countArgs = append(countArgs, serviceType)
		idx++
	}
	if isAdvertised != nil {
		clause := fmt.Sprintf(" AND is_advertised = $%d", idx)
		query += clause
		countQuery += clause
		args = append(args, *isAdvertised)
		countArgs = append(countArgs, *isAdvertised)
		idx++
	}
	if isActive != nil {
		clause := fmt.Sprintf(" AND is_active = $%d", idx)
		query += clause
		countQuery += clause
		args = append(args, *isActive)
		countArgs = append(countArgs, *isActive)
		idx++
	}

	var total int
	if err := d.pool.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count services: %w", err)
	}

	query += " ORDER BY created_at DESC"
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", idx, idx+1)
	args = append(args, limit, offset)

	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list services: %w", err)
	}
	defer rows.Close()

	var services []ServiceRecord
	for rows.Next() {
		var s ServiceRecord
		if err := rows.Scan(&s.ID, &s.SourceAccountID, &s.ServiceName, &s.ServiceType, &s.Port, &s.Host, &s.Domain,
			&s.TxtRecords, &s.IsAdvertised, &s.IsActive, &s.LastSeenAt, &s.Metadata, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan service: %w", err)
		}
		services = append(services, s)
	}
	return services, total, rows.Err()
}

// UpdateService updates an existing service by ID using dynamic SET clauses.
// Size-cap exception: single DB operation — 70L scan loop with struct mapping; splitting would fragment a single SQL query across files.
func (d *DB) UpdateService(id string, req UpdateServiceRequest) (*ServiceRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sets := []string{"updated_at = NOW()"}
	args := []interface{}{}
	idx := 1

	if req.ServiceName != nil {
		sets = append(sets, fmt.Sprintf("service_name = $%d", idx))
		args = append(args, *req.ServiceName)
		idx++
	}
	if req.ServiceType != nil {
		sets = append(sets, fmt.Sprintf("service_type = $%d", idx))
		args = append(args, *req.ServiceType)
		idx++
	}
	if req.Port != nil {
		sets = append(sets, fmt.Sprintf("port = $%d", idx))
		args = append(args, *req.Port)
		idx++
	}
	if req.Host != nil {
		sets = append(sets, fmt.Sprintf("host = $%d", idx))
		args = append(args, *req.Host)
		idx++
	}
	if req.Domain != nil {
		sets = append(sets, fmt.Sprintf("domain = $%d", idx))
		args = append(args, *req.Domain)
		idx++
	}
	if req.TxtRecords != nil {
		sets = append(sets, fmt.Sprintf("txt_records = $%d", idx))
		args = append(args, *req.TxtRecords)
		idx++
	}
	if req.IsActive != nil {
		sets = append(sets, fmt.Sprintf("is_active = $%d", idx))
		args = append(args, *req.IsActive)
		idx++
	}
	if req.Metadata != nil {
		sets = append(sets, fmt.Sprintf("metadata = $%d", idx))
		args = append(args, *req.Metadata)
		idx++
	}

	args = append(args, id, d.sourceAccountID)
	query := fmt.Sprintf(
		`UPDATE np_mdns_services SET %s WHERE id = $%d AND source_account_id = $%d
		 RETURNING id, source_account_id, service_name, service_type, port, host, domain,
			txt_records, is_advertised, is_active, last_seen_at, metadata, created_at, updated_at`,
		joinStrings(sets, ", "), idx, idx+1,
	)

	var s ServiceRecord
	err := d.pool.QueryRow(ctx, query, args...).Scan(
		&s.ID, &s.SourceAccountID, &s.ServiceName, &s.ServiceType, &s.Port, &s.Host, &s.Domain,
		&s.TxtRecords, &s.IsAdvertised, &s.IsActive, &s.LastSeenAt, &s.Metadata, &s.CreatedAt, &s.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update service: %w", err)
	}
	return &s, nil
}

// DeleteService removes a service by ID. Returns true if a row was deleted.
func (d *DB) DeleteService(id string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tag, err := d.pool.Exec(ctx,
		`DELETE FROM np_mdns_services WHERE id = $1 AND source_account_id = $2`, id, d.sourceAccountID)
	if err != nil {
		return false, fmt.Errorf("delete service: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// SetAdvertised toggles the is_advertised flag and updates last_seen_at.
