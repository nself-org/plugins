package internal

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
)

// sa extracts source_account_id from request headers (X-Source-Account-Id), defaulting to "primary".
func sa(r *http.Request) string {
	if v := r.Header.Get("X-Source-Account-Id"); v != "" {
		return v
	}
	if v := r.Header.Get("X-App-Id"); v != "" {
		return v
	}
	return "primary"
}

// writeJSON encodes v as JSON, sets Content-Type, and writes status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

// Handlers holds all route handler methods.
type Handlers struct {
	db  *DB
	cfg *Config
}

func NewHandlers(db *DB, cfg *Config) *Handlers {
	return &Handlers{db: db, cfg: cfg}
}

// HealthCheck GET /health
func (h *Handlers) HealthCheck(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"plugin":    "admin-api",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// Ready GET /ready
func (h *Handlers) Ready(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, _, _, _, err := h.db.HealthCheck(ctx)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"ready":     false,
			"plugin":    "admin-api",
			"error":     "database unavailable",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ready":     true,
		"plugin":    "admin-api",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// GetMetrics GET /api/v1/metrics
func (h *Handlers) GetMetrics(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, collectSystemMetrics())
}

// GetMetricsHistory GET /api/v1/metrics/history
func (h *Handlers) GetMetricsHistory(w http.ResponseWriter, r *http.Request) {
	period := r.URL.Query().Get("period")
	snapshots, err := h.db.ForAccount(sa(r)).ListSnapshots(r.Context(), period)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if snapshots == nil {
		snapshots = []*MetricsSnapshot{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"snapshots": snapshots, "count": len(snapshots)})
}

// CreateSnapshot POST /api/v1/metrics/snapshot
func (h *Handlers) CreateSnapshot(w http.ResponseWriter, r *http.Request) {
	var req CreateSnapshotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON"})
		return
	}
	snap, err := h.db.ForAccount(sa(r)).CreateSnapshot(r.Context(), &req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, snap)
}

// GetDashboard GET /api/v1/dashboard
func (h *Handlers) GetDashboard(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.db.ForAccount(sa(r)).GetDashboardConfig(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if cfg == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"source_account_id": sa(r),
			"config_key":        "dashboard",
			"config_value":      map[string]any{"widgets": []any{}, "layout": map[string]any{}},
		})
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// UpdateDashboard PUT /api/v1/dashboard
func (h *Handlers) UpdateDashboard(w http.ResponseWriter, r *http.Request) {
	var req UpdateDashboardRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON"})
		return
	}
	cfg, err := h.db.ForAccount(sa(r)).UpsertDashboardConfig(r.Context(), req.Widgets, req.Layout)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// HealthServices GET /api/v1/health/services
func (h *Handlers) HealthServices(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	latency, connCount, maxConn, version, err := h.db.HealthCheck(ctx)

	dbStatus := "healthy"
	var dbErr *string
	if err != nil {
		dbStatus = "unhealthy"
		msg := err.Error()
		dbErr = &msg
	}

	now := time.Now().UTC().Format(time.RFC3339)
	services := []ServiceHealth{
		{
			Name:      "admin-api",
			Status:    "healthy",
			LatencyMS: nil,
			LastCheck: now,
			Error:     nil,
		},
		{
			Name:      "database",
			Status:    dbStatus,
			LatencyMS: &latency,
			LastCheck: now,
			Error:     dbErr,
		},
	}

	_ = connCount
	_ = maxConn
	_ = version

	writeJSON(w, http.StatusOK, map[string]any{
		"services":  services,
		"timestamp": now,
	})
}

// GetLogs GET /api/v1/logs
func (h *Handlers) GetLogs(w http.ResponseWriter, r *http.Request) {
	level := r.URL.Query().Get("level")
	if level == "" {
		level = "info"
	}
	now := time.Now().UTC()
	logs := []LogEntry{
		{
			Timestamp: now.Format(time.RFC3339),
			Level:     "info",
			Message:   fmt.Sprintf("admin-api running on port %d", h.cfg.Port),
			Source:    "admin-api",
		},
	}
	// Filter by requested level
	var filtered []LogEntry
	for _, l := range logs {
		if level == "info" || l.Level == level {
			filtered = append(filtered, l)
		}
	}
	if filtered == nil {
		filtered = []LogEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"logs": filtered, "count": len(filtered)})
}

// GetAudit GET /api/v1/audit
func (h *Handlers) GetAudit(w http.ResponseWriter, r *http.Request) {
	limit := 100
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			offset = n
		}
	}
	entries, err := h.db.GetAuditLog(r.Context(), limit, offset)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if entries == nil {
		entries = []*AuditEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries, "count": len(entries)})
}

// CreateAudit POST /api/v1/audit
func (h *Handlers) CreateAudit(w http.ResponseWriter, r *http.Request) {
	var req AuditRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON"})
		return
	}
	if req.Action == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "action is required"})
		return
	}
	entry, err := h.db.CreateAuditEntry(r.Context(), &req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, entry)
}

// GetStats GET /api/v1/stats
func (h *Handlers) GetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.db.ForAccount(sa(r)).GetStats(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"plugin":    "admin-api",
		"version":   "1.0.0",
		"stats":     stats,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// RegisterRoutes attaches all routes to r.
func (h *Handlers) RegisterRoutes(r chi.Router) {
	r.Get("/health", h.HealthCheck)
	r.Get("/ready", h.Ready)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/metrics", h.GetMetrics)
		r.Get("/metrics/history", h.GetMetricsHistory)
		r.Post("/metrics/snapshot", h.CreateSnapshot)

		r.Get("/dashboard", h.GetDashboard)
		r.Put("/dashboard", h.UpdateDashboard)

		r.Get("/health/services", h.HealthServices)
		r.Get("/logs", h.GetLogs)

		r.Get("/audit", h.GetAudit)
		r.Post("/audit", h.CreateAudit)

		r.Get("/stats", h.GetStats)
	})
}

// collectSystemMetrics gathers live OS-level metrics.
func collectSystemMetrics() *SystemMetrics {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	var loadAvg [3]float64
	if info, err := readLoadAvg(); err == nil {
		loadAvg = info
	}

	var totalMem, freeMem uint64
	if info, err := readMemInfo(); err == nil {
		totalMem = info[0]
		freeMem = info[1]
	}

	var usedPct float64
	if totalMem > 0 {
		usedPct = math.Round(float64(totalMem-freeMem)/float64(totalMem)*10000) / 100
	}

	// CPU usage approximation via /proc/stat (Linux) — zero on other platforms
	cpuUsage := readCPUUsage()

	// Disk usage via syscall
	diskUsed, diskTotal, diskFree := readDiskUsage("/")

	return &SystemMetrics{
		CPU: CPUMetrics{
			UsagePercent:   cpuUsage,
			LoadAverage1m:  loadAvg[0],
			LoadAverage5m:  loadAvg[1],
			LoadAverage15m: loadAvg[2],
		},
		Memory: MemoryMetrics{
			UsedBytes:    totalMem - freeMem,
			TotalBytes:   totalMem,
			FreeBytes:    freeMem,
			UsagePercent: usedPct,
		},
		Disk: DiskMetrics{
			UsedBytes:    diskUsed,
			TotalBytes:   diskTotal,
			FreeBytes:    diskFree,
			UsagePercent: func() float64 {
				if diskTotal == 0 {
					return 0
				}
				return math.Round(float64(diskUsed)/float64(diskTotal)*10000) / 100
			}(),
		},
		Network: NetworkMetrics{},
		Process: ProcessMetrics{
			UptimeSeconds: 0,
			PID:           os.Getpid(),
			GoVersion:     runtime.Version(),
			Platform:      runtime.GOOS,
		},
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
}

func readLoadAvg() ([3]float64, error) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return [3]float64{}, err
	}
	var a, b, c float64
	fmt.Sscanf(string(data), "%f %f %f", &a, &b, &c)
	return [3]float64{a, b, c}, nil
}

func readMemInfo() ([2]uint64, error) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return [2]uint64{}, err
	}
	var total, free uint64
	for _, line := range splitLines(string(data)) {
		var key string
		var val uint64
		fmt.Sscanf(line, "%s %d", &key, &val)
		switch key {
		case "MemTotal:":
			total = val * 1024
		case "MemFree:":
			free = val * 1024
		}
	}
	return [2]uint64{total, free}, nil
}

func readCPUUsage() float64 {
	// Stateless snapshot — returns 0 (accurate delta requires two reads)
	return 0
}

func readDiskUsage(path string) (used, total, free uint64) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, 0
	}
	total = stat.Blocks * uint64(stat.Bsize)
	free = stat.Bfree * uint64(stat.Bsize)
	used = total - free
	return
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
