package internal

import (
	"context"
	"strconv"
)

// ---------------------------------------------------------------------------
// Statistics
// ---------------------------------------------------------------------------

// GetStatistics returns aggregate usage statistics.
func (d *DB) GetStatistics(ctx context.Context) (*Statistics, error) {
	stats := &Statistics{
		Providers:  []ProviderStat{},
		TopServers: []ServerStat{},
	}

	// Connections summary
	var totalConn, activeConn int
	err := d.pool.QueryRow(ctx,
		`SELECT
			COUNT(*),
			COUNT(CASE WHEN status = 'connected' THEN 1 END)
		FROM np_vpn_connections
		WHERE source_account_id = $1`,
		d.sourceAccountID).Scan(&totalConn, &activeConn)
	if err != nil {
		return nil, err
	}
	stats.TotalConnections = totalConn
	stats.ActiveConnections = activeConn

	// Downloads summary
	var totalDL, activeDL int
	var totalBytes *int64
	err = d.pool.QueryRow(ctx,
		`SELECT
			COUNT(*),
			COUNT(CASE WHEN status IN ('downloading', 'queued', 'connecting_vpn') THEN 1 END),
			SUM(bytes_downloaded)
		FROM np_vpn_downloads
		WHERE source_account_id = $1`,
		d.sourceAccountID).Scan(&totalDL, &activeDL, &totalBytes)
	if err != nil {
		return nil, err
	}
	stats.TotalDownloads = totalDL
	stats.ActiveDownloads = activeDL
	if totalBytes != nil {
		stats.TotalBytesDownloaded = strconv.FormatInt(*totalBytes, 10)
	} else {
		stats.TotalBytesDownloaded = "0"
	}

	// Provider stats
	provRows, err := d.pool.Query(ctx,
		`SELECT p.display_name, COUNT(c.id) AS total_connections,
			ROUND((COUNT(CASE WHEN c.error_message IS NULL THEN 1 END)::NUMERIC /
				   NULLIF(COUNT(c.id), 0) * 100), 2) AS success_rate_percent
		FROM np_vpn_providers p
		LEFT JOIN np_vpn_connections c ON p.id = c.provider_id AND c.source_account_id = $1
		WHERE p.source_account_id = $1
		GROUP BY p.id, p.display_name
		ORDER BY COUNT(c.id) DESC
		LIMIT 10`,
		d.sourceAccountID)
	if err != nil {
		return nil, err
	}
	defer provRows.Close()

	for provRows.Next() {
		var ps ProviderStat
		var successRate *float64
		if err := provRows.Scan(&ps.Provider, &ps.Connections, &successRate); err != nil {
			return nil, err
		}
		if successRate != nil {
			ps.UptimePercentage = *successRate
		}
		stats.Providers = append(stats.Providers, ps)
	}
	if err := provRows.Err(); err != nil {
		return nil, err
	}

	// Top servers
	srvRows, err := d.pool.Query(ctx,
		`SELECT s.hostname, s.provider_id, s.country_code,
			COUNT(DISTINCT c.id) AS total_connections,
			AVG(sp.download_speed_mbps) AS avg_download_speed
		FROM np_vpn_servers s
		LEFT JOIN np_vpn_connections c ON s.id = c.server_id AND c.source_account_id = $1
		LEFT JOIN np_vpn_server_performance sp ON s.id = sp.server_id AND sp.source_account_id = $1
		WHERE s.source_account_id = $1
		GROUP BY s.id, s.hostname, s.provider_id, s.country_code
		ORDER BY COUNT(DISTINCT c.id) DESC, AVG(sp.download_speed_mbps) DESC
		LIMIT 10`,
		d.sourceAccountID)
	if err != nil {
		return nil, err
	}
	defer srvRows.Close()

	for srvRows.Next() {
		var ss ServerStat
		var avgSpeed *float64
		if err := srvRows.Scan(&ss.Server, &ss.Provider, &ss.Country, &ss.Connections, &avgSpeed); err != nil {
			return nil, err
		}
		if avgSpeed != nil {
			ss.AvgSpeedMbps = *avgSpeed
		}
		stats.TopServers = append(stats.TopServers, ss)
	}
	if err := srvRows.Err(); err != nil {
		return nil, err
	}

	return stats, nil
}

