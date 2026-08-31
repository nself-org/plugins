package internal

import "time"

// MetricsSnapshot maps to np_admin_metrics_snapshots.
type MetricsSnapshot struct {
	ID                  string         `json:"id"`
	SourceAccountID     string         `json:"source_account_id"`
	MetricType          string         `json:"metric_type"`
	CPUUsagePercent     *float64       `json:"cpu_usage_percent"`
	MemoryUsedBytes     *int64         `json:"memory_used_bytes"`
	MemoryTotalBytes    *int64         `json:"memory_total_bytes"`
	DiskUsedBytes       *int64         `json:"disk_used_bytes"`
	DiskTotalBytes      *int64         `json:"disk_total_bytes"`
	ActiveConnections   *int32         `json:"active_connections"`
	RequestCount        *int32         `json:"request_count"`
	ErrorCount          *int32         `json:"error_count"`
	AvgResponseTimeMS   *float64       `json:"avg_response_time_ms"`
	ActiveSessions      *int32         `json:"active_sessions"`
	Metadata            map[string]any `json:"metadata"`
	CreatedAt           time.Time      `json:"created_at"`
}

// DashboardConfig maps to np_admin_dashboard_config.
type DashboardConfig struct {
	ID              string         `json:"id"`
	SourceAccountID string         `json:"source_account_id"`
	ConfigKey       string         `json:"config_key"`
	ConfigValue     map[string]any `json:"config_value"`
	Description     *string        `json:"description"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

// CreateSnapshotRequest is the POST /api/v1/metrics/snapshot body.
type CreateSnapshotRequest struct {
	MetricType        string         `json:"metric_type"`
	CPUUsagePercent   *float64       `json:"cpu_usage_percent,omitempty"`
	MemoryUsedBytes   *int64         `json:"memory_used_bytes,omitempty"`
	MemoryTotalBytes  *int64         `json:"memory_total_bytes,omitempty"`
	DiskUsedBytes     *int64         `json:"disk_used_bytes,omitempty"`
	DiskTotalBytes    *int64         `json:"disk_total_bytes,omitempty"`
	ActiveConnections *int32         `json:"active_connections,omitempty"`
	RequestCount      *int32         `json:"request_count,omitempty"`
	ErrorCount        *int32         `json:"error_count,omitempty"`
	AvgResponseTimeMS *float64       `json:"avg_response_time_ms,omitempty"`
	ActiveSessions    *int32         `json:"active_sessions,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
}

// UpdateDashboardRequest is the PUT /api/v1/dashboard body.
type UpdateDashboardRequest struct {
	Widgets map[string]any `json:"widgets"`
	Layout  map[string]any `json:"layout"`
}

// AuditRequest is the POST /api/v1/audit body.
type AuditRequest struct {
	Action       string         `json:"action"`
	ResourceType string         `json:"resource_type"`
	ResourceID   string         `json:"resource_id"`
	UserID       string         `json:"user_id"`
	Details      map[string]any `json:"details,omitempty"`
}

// AuditEntry is a single audit log row.
type AuditEntry struct {
	ID           string         `json:"id"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resource_type"`
	ResourceID   string         `json:"resource_id"`
	UserID       string         `json:"user_id"`
	Details      map[string]any `json:"details"`
	CreatedAt    time.Time      `json:"created_at"`
}

// SystemMetrics is the live metrics response.
type SystemMetrics struct {
	CPU       CPUMetrics     `json:"cpu"`
	Memory    MemoryMetrics  `json:"memory"`
	Disk      DiskMetrics    `json:"disk"`
	Network   NetworkMetrics `json:"network"`
	Process   ProcessMetrics `json:"process"`
	Timestamp string         `json:"timestamp"`
}

type CPUMetrics struct {
	UsagePercent    float64 `json:"usage_percent"`
	LoadAverage1m   float64 `json:"load_average_1m"`
	LoadAverage5m   float64 `json:"load_average_5m"`
	LoadAverage15m  float64 `json:"load_average_15m"`
}

type MemoryMetrics struct {
	UsedBytes    uint64  `json:"used_bytes"`
	TotalBytes   uint64  `json:"total_bytes"`
	FreeBytes    uint64  `json:"free_bytes"`
	UsagePercent float64 `json:"usage_percent"`
}

type DiskMetrics struct {
	UsedBytes    uint64  `json:"used_bytes"`
	TotalBytes   uint64  `json:"total_bytes"`
	FreeBytes    uint64  `json:"free_bytes"`
	UsagePercent float64 `json:"usage_percent"`
}

type NetworkMetrics struct {
	ActiveConnections  int     `json:"active_connections"`
	RequestsPerMinute  float64 `json:"requests_per_minute"`
	AvgResponseTimeMS  float64 `json:"avg_response_time_ms"`
	ErrorRatePercent   float64 `json:"error_rate_percent"`
}

type ProcessMetrics struct {
	UptimeSeconds float64 `json:"uptime_seconds"`
	PID           int     `json:"pid"`
	GoVersion     string  `json:"go_version"`
	Platform      string  `json:"platform"`
}

// DashboardStats are aggregate stats.
type DashboardStats struct {
	SnapshotsTotal      int64    `json:"snapshots_total"`
	SnapshotsToday      int64    `json:"snapshots_today"`
	OldestSnapshot      *string  `json:"oldest_snapshot"`
	NewestSnapshot      *string  `json:"newest_snapshot"`
	ConfigEntries       int64    `json:"config_entries"`
	AvgCPU24h           *float64 `json:"avg_cpu_24h"`
	AvgMemory24h        *float64 `json:"avg_memory_24h"`
	PeakConnections24h  *int64   `json:"peak_connections_24h"`
	TotalRequests24h    *int64   `json:"total_requests_24h"`
	TotalErrors24h      *int64   `json:"total_errors_24h"`
}

// ServiceHealth is a single service health entry.
type ServiceHealth struct {
	Name      string   `json:"name"`
	Status    string   `json:"status"`
	LatencyMS *float64 `json:"latency_ms"`
	LastCheck string   `json:"last_check"`
	Error     *string  `json:"error"`
}

// LogEntry is a system log entry.
type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	Source    string `json:"source"`
}
