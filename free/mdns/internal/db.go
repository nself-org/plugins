package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps a pgxpool.Pool with mDNS table operations.
type DB struct {
	pool            *pgxpool.Pool
	sourceAccountID string
}

// NewDB creates a new DB wrapper with source_account_id defaulting to "primary".
func NewDB(pool *pgxpool.Pool) *DB {
	return &DB{pool: pool, sourceAccountID: "primary"}
}

// ForSourceAccount returns a new DB scoped to a specific source_account_id.
func (d *DB) ForSourceAccount(id string) *DB {
	return &DB{pool: d.pool, sourceAccountID: id}
}

// InitSchema creates tables and indexes if they do not exist.
func (d *DB) InitSchema() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	schema := `
CREATE TABLE IF NOT EXISTS np_mdns_services (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    service_name VARCHAR(255) NOT NULL,
    service_type VARCHAR(128) NOT NULL DEFAULT '_ntv._tcp',
    port INTEGER NOT NULL,
    host VARCHAR(255) NOT NULL DEFAULT 'localhost',
    domain VARCHAR(128) NOT NULL DEFAULT 'local',
    txt_records JSONB DEFAULT '{}',
    is_advertised BOOLEAN DEFAULT false,
    is_active BOOLEAN DEFAULT true,
    last_seen_at TIMESTAMPTZ DEFAULT NOW(),
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(source_account_id, service_name, service_type)
);
CREATE INDEX IF NOT EXISTS idx_np_mdns_services_account ON np_mdns_services(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_mdns_services_type ON np_mdns_services(service_type);
CREATE INDEX IF NOT EXISTS idx_np_mdns_services_active ON np_mdns_services(is_active);
CREATE INDEX IF NOT EXISTS idx_np_mdns_services_advertised ON np_mdns_services(is_advertised);

CREATE TABLE IF NOT EXISTS np_mdns_discovery_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    service_type VARCHAR(128) NOT NULL,
    service_name VARCHAR(255) NOT NULL,
    host VARCHAR(255) NOT NULL,
    port INTEGER NOT NULL,
    addresses TEXT[] DEFAULT '{}',
    txt_records JSONB DEFAULT '{}',
    discovered_at TIMESTAMPTZ DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ DEFAULT NOW(),
    is_available BOOLEAN DEFAULT true,
    metadata JSONB DEFAULT '{}',
    UNIQUE(source_account_id, service_name, service_type, host)
);
CREATE INDEX IF NOT EXISTS idx_np_mdns_discovery_account ON np_mdns_discovery_log(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_mdns_discovery_type ON np_mdns_discovery_log(service_type);
CREATE INDEX IF NOT EXISTS idx_np_mdns_discovery_available ON np_mdns_discovery_log(is_available);
`
	_, err := d.pool.Exec(ctx, schema)
	return err
}

// Ping checks database connectivity.
func (d *DB) Ping() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return d.pool.Ping(ctx)
}

// --- Service CRUD ---

// CreateService inserts a new mDNS service record.
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
func (d *DB) SetAdvertised(id string, advertised bool) (*ServiceRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var s ServiceRecord
	err := d.pool.QueryRow(ctx,
		`UPDATE np_mdns_services SET is_advertised = $1, last_seen_at = NOW(), updated_at = NOW()
		 WHERE id = $2 AND source_account_id = $3
		 RETURNING id, source_account_id, service_name, service_type, port, host, domain,
			txt_records, is_advertised, is_active, last_seen_at, metadata, created_at, updated_at`,
		advertised, id, d.sourceAccountID,
	).Scan(&s.ID, &s.SourceAccountID, &s.ServiceName, &s.ServiceType, &s.Port, &s.Host, &s.Domain,
		&s.TxtRecords, &s.IsAdvertised, &s.IsActive, &s.LastSeenAt, &s.Metadata, &s.CreatedAt, &s.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("set advertised: %w", err)
	}
	return &s, nil
}

// --- Discovery operations ---

// UpsertDiscovery inserts or updates a discovery log entry.
func (d *DB) UpsertDiscovery(entry DiscoverEntry) (*DiscoveryLogRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	txtRecords := json.RawMessage(`{}`)
	if entry.TxtRecords != nil {
		txtRecords = *entry.TxtRecords
	}
	metadata := json.RawMessage(`{}`)
	if entry.Metadata != nil {
		metadata = *entry.Metadata
	}
	addresses := entry.Addresses
	if addresses == nil {
		addresses = []string{}
	}

	var rec DiscoveryLogRecord
	err := d.pool.QueryRow(ctx,
		`INSERT INTO np_mdns_discovery_log
			(source_account_id, service_type, service_name, host, port, addresses, txt_records, metadata)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (source_account_id, service_name, service_type, host) DO UPDATE
			SET port = EXCLUDED.port,
				addresses = EXCLUDED.addresses,
				txt_records = EXCLUDED.txt_records,
				metadata = EXCLUDED.metadata,
				last_seen_at = NOW(),
				is_available = true
		 RETURNING id, source_account_id, service_type, service_name, host, port,
			addresses, txt_records, discovered_at, last_seen_at, is_available, metadata`,
		d.sourceAccountID, entry.ServiceType, entry.ServiceName, entry.Host, entry.Port,
		addresses, txtRecords, metadata,
	).Scan(&rec.ID, &rec.SourceAccountID, &rec.ServiceType, &rec.ServiceName, &rec.Host, &rec.Port,
		&rec.Addresses, &rec.TxtRecords, &rec.DiscoveredAt, &rec.LastSeenAt, &rec.IsAvailable, &rec.Metadata)
	if err != nil {
		return nil, fmt.Errorf("upsert discovery: %w", err)
	}
	return &rec, nil
}

// ListDiscoveries returns discovery log entries with optional filtering and pagination.
func (d *DB) ListDiscoveries(serviceType string, isAvailable *bool, limit, offset int) ([]DiscoveryLogRecord, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `SELECT id, source_account_id, service_type, service_name, host, port,
		addresses, txt_records, discovered_at, last_seen_at, is_available, metadata
		FROM np_mdns_discovery_log WHERE source_account_id = $1`
	countQuery := `SELECT COUNT(*) FROM np_mdns_discovery_log WHERE source_account_id = $1`
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
	if isAvailable != nil {
		clause := fmt.Sprintf(" AND is_available = $%d", idx)
		query += clause
		countQuery += clause
		args = append(args, *isAvailable)
		countArgs = append(countArgs, *isAvailable)
		idx++
	}

	var total int
	if err := d.pool.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count discoveries: %w", err)
	}

	query += " ORDER BY last_seen_at DESC"
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", idx, idx+1)
	args = append(args, limit, offset)

	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list discoveries: %w", err)
	}
	defer rows.Close()

	var records []DiscoveryLogRecord
	for rows.Next() {
		var rec DiscoveryLogRecord
		if err := rows.Scan(&rec.ID, &rec.SourceAccountID, &rec.ServiceType, &rec.ServiceName, &rec.Host, &rec.Port,
			&rec.Addresses, &rec.TxtRecords, &rec.DiscoveredAt, &rec.LastSeenAt, &rec.IsAvailable, &rec.Metadata); err != nil {
			return nil, 0, fmt.Errorf("scan discovery: %w", err)
		}
		records = append(records, rec)
	}
	return records, total, rows.Err()
}

// MarkUnavailable marks a discovery log entry as unavailable by ID.
func (d *DB) MarkUnavailable(id string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tag, err := d.pool.Exec(ctx,
		`UPDATE np_mdns_discovery_log SET is_available = false, last_seen_at = NOW()
		 WHERE id = $1 AND source_account_id = $2`, id, d.sourceAccountID)
	if err != nil {
		return false, fmt.Errorf("mark unavailable: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// --- Stats ---

// GetStats returns aggregate statistics across both tables.
func (d *DB) GetStats() (*MdnsStats, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stats MdnsStats

	err := d.pool.QueryRow(ctx,
		`SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE is_active = true),
			COUNT(*) FILTER (WHERE is_advertised = true)
		 FROM np_mdns_services WHERE source_account_id = $1`, d.sourceAccountID,
	).Scan(&stats.TotalServices, &stats.ActiveServices, &stats.AdvertisedServices)
	if err != nil {
		return nil, fmt.Errorf("stats services: %w", err)
	}

	err = d.pool.QueryRow(ctx,
		`SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE is_available = true)
		 FROM np_mdns_discovery_log WHERE source_account_id = $1`, d.sourceAccountID,
	).Scan(&stats.TotalDiscovered, &stats.AvailableDiscovered)
	if err != nil {
		return nil, fmt.Errorf("stats discoveries: %w", err)
	}

	return &stats, nil
}

// joinStrings joins a slice of strings with a separator.
func joinStrings(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for _, p := range parts[1:] {
		result += sep + p
	}
	return result
}
