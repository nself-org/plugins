package internal

import (
	"context"
	"encoding/json"
	"fmt"
	pgx "github.com/jackc/pgx/v5"
)

// ---------------------------------------------------------------------------
// Server operations
// ---------------------------------------------------------------------------

// GetServers returns servers matching the given filters, scoped to the current account.
func (d *DB) GetServers(ctx context.Context, f ServerFilter) ([]Server, error) {
	conditions := []string{"source_account_id = $1"}
	args := []interface{}{d.sourceAccountID}
	argIdx := 2

	if f.Provider != "" {
		conditions = append(conditions, fmt.Sprintf("provider_id = $%d", argIdx))
		args = append(args, f.Provider)
		argIdx++
	}
	if f.Country != "" {
		conditions = append(conditions, fmt.Sprintf("country_code = $%d", argIdx))
		args = append(args, f.Country)
		argIdx++
	}
	if f.P2POnly {
		conditions = append(conditions, "p2p_supported = true")
	}
	if f.PortForwarding {
		conditions = append(conditions, "port_forwarding_supported = true")
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}

	query := fmt.Sprintf(`
		SELECT id, provider_id, hostname, ip_address, ipv6_address, country_code, country_name,
			city, region, latitude, longitude, p2p_supported, port_forwarding_supported,
			protocols, load, capacity, status, features, public_key, endpoint_port,
			owned, metadata, source_account_id, last_seen, created_at, updated_at
		FROM np_vpn_servers
		WHERE %s
		ORDER BY load ASC NULLS LAST, created_at DESC
		LIMIT $%d`,
		joinAnd(conditions), argIdx)
	args = append(args, limit)

	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Server
	for rows.Next() {
		s, err := scanServer(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

func scanServer(rows pgx.Rows) (Server, error) {
	var s Server
	var metaJSON []byte
	err := rows.Scan(
		&s.ID, &s.ProviderID, &s.Hostname, &s.IPAddress, &s.IPv6Address,
		&s.CountryCode, &s.CountryName, &s.City, &s.Region,
		&s.Latitude, &s.Longitude, &s.P2PSupported, &s.PortForwardingSupported,
		&s.Protocols, &s.Load, &s.Capacity, &s.Status, &s.Features,
		&s.PublicKey, &s.EndpointPort, &s.Owned, &metaJSON,
		&s.SourceAccountID, &s.LastSeen, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return Server{}, err
	}
	s.Metadata = make(map[string]interface{})
	if len(metaJSON) > 0 {
		_ = json.Unmarshal(metaJSON, &s.Metadata)
	}
	if s.Protocols == nil {
		s.Protocols = []string{}
	}
	if s.Features == nil {
		s.Features = []string{}
	}
	return s, nil
}

