package internal

import (
	"context"
	"fmt"
	"time"
)

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
