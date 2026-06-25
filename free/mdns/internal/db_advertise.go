package internal

import (
	pgx "github.com/jackc/pgx/v5"
	"context"
	"encoding/json"
	"fmt"
	"time"
)

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
