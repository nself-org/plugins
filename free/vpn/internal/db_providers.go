package internal

import (
	"context"
	"encoding/json"
	pgx "github.com/jackc/pgx/v5"
)

// ---------------------------------------------------------------------------
// Provider operations
// ---------------------------------------------------------------------------

// GetAllProviders returns all providers scoped to the current account.
func (d *DB) GetAllProviders(ctx context.Context) ([]Provider, error) {
	rows, err := d.pool.Query(ctx,
		`SELECT id, name, display_name, cli_available, cli_command, api_available, api_endpoint,
			port_forwarding_supported, p2p_all_servers, p2p_server_count, total_servers,
			total_countries, wireguard_supported, openvpn_supported, kill_switch_available,
			split_tunneling_available, config, source_account_id, created_at, updated_at
		FROM np_vpn_providers WHERE source_account_id = $1 ORDER BY name`,
		d.sourceAccountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Provider
	for rows.Next() {
		var p Provider
		var configJSON []byte
		if err := rows.Scan(
			&p.ID, &p.Name, &p.DisplayName, &p.CLIAvailable, &p.CLICommand,
			&p.APIAvailable, &p.APIEndpoint, &p.PortForwardingSupported,
			&p.P2PAllServers, &p.P2PServerCount, &p.TotalServers, &p.TotalCountries,
			&p.WireguardSupported, &p.OpenVPNSupported, &p.KillSwitchAvailable,
			&p.SplitTunnelingAvailable, &configJSON, &p.SourceAccountID,
			&p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, err
		}
		p.Config = make(map[string]interface{})
		if len(configJSON) > 0 {
			_ = json.Unmarshal(configJSON, &p.Config)
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

// GetProvider returns a single provider by ID, scoped to the current account.
func (d *DB) GetProvider(ctx context.Context, id string) (*Provider, error) {
	var p Provider
	var configJSON []byte
	err := d.pool.QueryRow(ctx,
		`SELECT id, name, display_name, cli_available, cli_command, api_available, api_endpoint,
			port_forwarding_supported, p2p_all_servers, p2p_server_count, total_servers,
			total_countries, wireguard_supported, openvpn_supported, kill_switch_available,
			split_tunneling_available, config, source_account_id, created_at, updated_at
		FROM np_vpn_providers WHERE id = $1 AND source_account_id = $2`,
		id, d.sourceAccountID,
	).Scan(
		&p.ID, &p.Name, &p.DisplayName, &p.CLIAvailable, &p.CLICommand,
		&p.APIAvailable, &p.APIEndpoint, &p.PortForwardingSupported,
		&p.P2PAllServers, &p.P2PServerCount, &p.TotalServers, &p.TotalCountries,
		&p.WireguardSupported, &p.OpenVPNSupported, &p.KillSwitchAvailable,
		&p.SplitTunnelingAvailable, &configJSON, &p.SourceAccountID,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.Config = make(map[string]interface{})
	if len(configJSON) > 0 {
		_ = json.Unmarshal(configJSON, &p.Config)
	}
	return &p, nil
}

