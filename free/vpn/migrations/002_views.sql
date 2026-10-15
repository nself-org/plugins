-- VPN reporting views, versioned separately to preserve the initial migration checksum.
-- Public qualification is required because the migration runner pins search_path.

CREATE OR REPLACE VIEW public.vpn_active_connections AS
SELECT
	c.id,
	c.provider_id,
	c.source_account_id,
	p.display_name AS provider_name,
	s.hostname AS server,
	s.country_code,
	s.city,
	c.vpn_ip,
	c.interface_name,
	c.protocol,
	c.connected_at,
	EXTRACT(EPOCH FROM (NOW() - c.connected_at))::INTEGER AS uptime_seconds,
	c.bytes_sent,
	c.bytes_received,
	c.port_forwarded,
	c.kill_switch_enabled
FROM public.np_vpn_connections c
JOIN public.np_vpn_providers p ON c.provider_id = p.id AND c.source_account_id = p.source_account_id
LEFT JOIN public.np_vpn_servers s ON c.server_id = s.id AND c.source_account_id = s.source_account_id
WHERE c.status = 'connected'
ORDER BY c.connected_at DESC;

CREATE OR REPLACE VIEW public.vpn_server_stats AS
SELECT
	s.id,
	s.provider_id,
	s.source_account_id,
	s.hostname,
	s.country_code,
	s.city,
	s.p2p_supported,
	s.port_forwarding_supported,
	s.load,
	s.status,
	COUNT(DISTINCT c.id) AS total_connections,
	AVG(sp.download_speed_mbps) AS avg_download_speed,
	AVG(sp.ping_ms) AS avg_ping,
	AVG(sp.success_rate) AS avg_success_rate,
	MAX(c.connected_at) AS last_used
FROM public.np_vpn_servers s
LEFT JOIN public.np_vpn_connections c ON s.id = c.server_id AND s.source_account_id = c.source_account_id
LEFT JOIN public.np_vpn_server_performance sp ON s.id = sp.server_id AND s.source_account_id = sp.source_account_id
GROUP BY s.id
ORDER BY total_connections DESC, avg_download_speed DESC;

CREATE OR REPLACE VIEW public.vpn_download_history AS
SELECT
	d.id,
	d.name,
	d.info_hash,
	d.status,
	d.progress,
	d.bytes_downloaded,
	d.bytes_total,
	d.requested_by,
	d.source_account_id,
	p.display_name AS provider_name,
	s.hostname AS server,
	s.country_code,
	d.started_at,
	d.completed_at,
	EXTRACT(EPOCH FROM (COALESCE(d.completed_at, NOW()) - d.started_at))::INTEGER AS duration_seconds,
	d.created_at
FROM public.np_vpn_downloads d
JOIN public.np_vpn_providers p ON d.provider_id = p.id AND d.source_account_id = p.source_account_id
LEFT JOIN public.np_vpn_servers s ON d.server_id = s.id AND d.source_account_id = s.source_account_id
ORDER BY d.created_at DESC;

CREATE OR REPLACE VIEW public.vpn_provider_uptime AS
SELECT
	p.id,
	p.source_account_id,
	p.display_name,
	COUNT(DISTINCT c.id) AS total_connections,
	SUM(CASE WHEN c.status = 'connected' THEN 1 ELSE 0 END) AS active_connections,
	SUM(c.duration_seconds) AS total_uptime_seconds,
	AVG(c.duration_seconds) AS avg_session_duration_seconds,
	ROUND((COUNT(CASE WHEN c.error_message IS NULL THEN 1 END)::NUMERIC /
		   NULLIF(COUNT(c.id), 0) * 100), 2) AS success_rate_percent
FROM public.np_vpn_providers p
LEFT JOIN public.np_vpn_connections c ON p.id = c.provider_id AND p.source_account_id = c.source_account_id
GROUP BY p.id
ORDER BY total_connections DESC;
