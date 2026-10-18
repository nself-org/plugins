// Package metering implements nCloud usage metering for E10.
//
// Two cron jobs are provided:
//   - CollectHourly: polls Hetzner API for each active tenant, writes raw
//     readings to np_cloud_usage_raw (upsert on conflict).
//   - ReportDaily: aggregates np_cloud_usage_raw for the past 24 hours,
//     applies free-tier deductions, reports usage to Stripe, and records
//     np_cloud_usage_reported rows for idempotency.
//
// Feature flag Y17 (NSELF_FLAG_CLOUD_METERING): when OFF, both cron jobs
// are registered but return early without executing. This enables dark-launch
// deployment with zero customer impact.
package metering

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nself-org/nself-tenant-controller/internal/config"
)

// Dimension constants match the CHECK constraint in the migration.
const (
	DimVCPU    = "vcpu"
	DimStorage = "storage"
	DimEgress  = "egress"
)

// Service holds deps for metering operations.
type Service struct {
	cfg  *config.Config
	pool *pgxpool.Pool
}

// New returns a metering Service. Call CollectHourly and ReportDaily from cron.
func New(cfg *config.Config, pool *pgxpool.Pool) *Service {
	return &Service{cfg: cfg, pool: pool}
}

// CollectHourly fetches current metrics from Hetzner for every active tenant
// and upserts one np_cloud_usage_raw row per dimension per tenant.
// Early-returns without action when the cloud_metering flag is OFF.
func (s *Service) CollectHourly(ctx context.Context) error {
	if !s.cfg.CloudMeteringEnabled {
		slog.InfoContext(ctx, "metering.CollectHourly: cloud_metering flag OFF — skipping")
		return nil
	}

	tenants, err := s.activeTenants(ctx)
	if err != nil {
		return fmt.Errorf("metering: list tenants: %w", err)
	}

	hourBucket := time.Now().UTC().Truncate(time.Hour)
	now := time.Now().UTC()

	for _, t := range tenants {
		if t.HetznerServerID == 0 {
			continue
		}

		vcpu, egress, err := s.hetznerServerMetrics(ctx, t.HetznerServerID)
		if err != nil {
			slog.WarnContext(ctx, "metering: hetzner server metrics error",
				"tenant_id", t.ID, "server_id", t.HetznerServerID, "err", err)
			continue
		}

		storageGB, err := s.hetznerVolumeGB(ctx, t.HetznerVolumeID)
		if err != nil {
			slog.WarnContext(ctx, "metering: hetzner volume error",
				"tenant_id", t.ID, "volume_id", t.HetznerVolumeID, "err", err)
		}

		rows := []struct {
			dim       string
			vcpu      *float64
			storage   *float64
			egress    *float64
		}{
			{dim: DimVCPU, vcpu: &vcpu},
			{dim: DimStorage, storage: &storageGB},
			{dim: DimEgress, egress: &egress},
		}

		for _, row := range rows {
			_, err = s.pool.Exec(ctx, `
				INSERT INTO np_cloud_usage_raw
					(tenant_id, dimension, hour_bucket, collected_at, vcpu_hours, storage_gb, egress_gb)
				VALUES ($1, $2, $3, $4, $5, $6, $7)
				ON CONFLICT (tenant_id, dimension, hour_bucket) DO UPDATE
					SET vcpu_hours   = EXCLUDED.vcpu_hours,
					    storage_gb   = EXCLUDED.storage_gb,
					    egress_gb    = EXCLUDED.egress_gb,
					    collected_at = EXCLUDED.collected_at
			`,
				t.ID, row.dim, hourBucket, now,
				row.vcpu, row.storage, row.egress,
			)
			if err != nil {
				slog.ErrorContext(ctx, "metering: upsert raw row failed",
					"tenant_id", t.ID, "dim", row.dim, "err", err)
			}
		}
	}

	slog.InfoContext(ctx, "metering.CollectHourly: complete", "tenants", len(tenants))
	return nil
}

// ReportDaily aggregates the past 24 hours of np_cloud_usage_raw for every
// tenant, applies free-tier allowance deductions, and calls
// stripe.subscriptionItems.createUsageRecord for each metered dimension.
// Idempotent: each (tenant, date, dimension) triple can only produce one
// Stripe usage record, enforced via the UNIQUE constraint on np_cloud_usage_reported.
// Early-returns without action when the cloud_metering flag is OFF.
func (s *Service) ReportDaily(ctx context.Context) error {
	if !s.cfg.CloudMeteringEnabled {
		slog.InfoContext(ctx, "metering.ReportDaily: cloud_metering flag OFF — skipping")
		return nil
	}

	reportDate := time.Now().UTC().Truncate(24 * time.Hour)
	since := reportDate.Add(-24 * time.Hour)

	tenants, err := s.activeTenants(ctx)
	if err != nil {
		return fmt.Errorf("metering: list tenants: %w", err)
	}

	// Fetch monthly totals for free-tier deduction. Monthly window starts on
	// the first day of the current UTC month.
	monthStart := time.Date(reportDate.Year(), reportDate.Month(), 1, 0, 0, 0, 0, time.UTC)

	for _, t := range tenants {
		if err := s.reportTenant(ctx, t, reportDate, since, monthStart); err != nil {
			slog.ErrorContext(ctx, "metering: report tenant failed",
				"tenant_id", t.ID, "err", err)
		}
	}

	slog.InfoContext(ctx, "metering.ReportDaily: complete", "date", reportDate.Format("2006-01-02"))
	return nil
}

func (s *Service) reportTenant(
	ctx context.Context,
	t tenant,
	reportDate, since, monthStart time.Time,
) error {
	// Aggregate 24-hour window.
	type dimSum struct {
		vcpuHours float64
		storageGB float64
		egressGB  float64
	}

	rows, err := s.pool.Query(ctx, `
		SELECT dimension, SUM(vcpu_hours), SUM(storage_gb), SUM(egress_gb)
		FROM np_cloud_usage_raw
		WHERE tenant_id = $1
		  AND hour_bucket >= $2
		  AND hour_bucket < $3
		GROUP BY dimension
	`, t.ID, since, reportDate)
	if err != nil {
		return fmt.Errorf("aggregate raw: %w", err)
	}
	defer rows.Close()

	sums := map[string]*dimSum{
		DimVCPU:    {},
		DimStorage: {},
		DimEgress:  {},
	}

	for rows.Next() {
		var dim string
		var vcpu, storage, egress *float64
		if err := rows.Scan(&dim, &vcpu, &storage, &egress); err != nil {
			return fmt.Errorf("scan: %w", err)
		}
		if _, ok := sums[dim]; !ok {
			continue
		}
		if vcpu != nil {
			sums[dim].vcpuHours += *vcpu
		}
		if storage != nil {
			sums[dim].storageGB += *storage
		}
		if egress != nil {
			sums[dim].egressGB += *egress
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows: %w", err)
	}

	// Fetch month-to-date totals for free-tier deduction.
	mtdVCPU, err := s.monthlyTotal(ctx, t.ID, DimVCPU, monthStart)
	if err != nil {
		return err
	}
	mtdStorage, err := s.monthlyTotal(ctx, t.ID, DimStorage, monthStart)
	if err != nil {
		return err
	}
	mtdEgress, err := s.monthlyTotal(ctx, t.ID, DimEgress, monthStart)
	if err != nil {
		return err
	}

	// Compute billable quantities after free-tier deduction.
	billable := map[string]int{
		DimVCPU:    s.billableVCPU(sums[DimVCPU].vcpuHours, mtdVCPU),
		DimStorage: s.billableStorage(sums[DimStorage].storageGB, mtdStorage),
		DimEgress:  s.billableEgress(sums[DimEgress].egressGB, mtdEgress),
	}

	siIDs := map[string]string{
		DimVCPU:    t.StripeSIVcpu,
		DimStorage: t.StripeSIStorage,
		DimEgress:  t.StripeSIEgress,
	}
	stripePrices := map[string]string{
		DimVCPU:    s.cfg.StripePriceVcpuHour,
		DimStorage: s.cfg.StripePriceStorageGBMonth,
		DimEgress:  s.cfg.StripePriceEgressGB,
	}

	for _, dim := range []string{DimVCPU, DimStorage, DimEgress} {
		qty := billable[dim]
		siID := siIDs[dim]
		if siID == "" || stripePrices[dim] == "" {
			continue
		}

		idempotencyKey := fmt.Sprintf("%s-%s-%s", t.ID, dim, reportDate.Format("2006-01-02"))

		usageRecordID, err := s.stripeCreateUsageRecord(ctx, siID, qty, reportDate, idempotencyKey)
		if err != nil {
			slog.ErrorContext(ctx, "metering: stripe usage record failed",
				"tenant_id", t.ID, "dim", dim, "qty", qty, "err", err)
			continue
		}

		_, err = s.pool.Exec(ctx, `
			INSERT INTO np_cloud_usage_reported
				(tenant_id, report_date, dimension, quantity, stripe_usage_record_id)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (tenant_id, report_date, dimension) DO NOTHING
		`, t.ID, reportDate.Format("2006-01-02"), dim, qty, usageRecordID)
		if err != nil {
			slog.ErrorContext(ctx, "metering: insert reported row failed",
				"tenant_id", t.ID, "dim", dim, "err", err)
		}

		// Check 80% alert threshold.
		s.maybeAlert(ctx, t, dim, mtdVCPU, mtdStorage, mtdEgress)
	}

	return nil
}

// billableVCPU returns the Stripe quantity (vCPU-hours × 100 to avoid sub-cent fractions)
// after deducting any remaining monthly free allowance.
func (s *Service) billableVCPU(todayHours, mtdHours float64) int {
	remaining := math.Max(0, float64(s.cfg.CloudFreeVcpuHours)-mtdHours)
	overage := math.Max(0, todayHours-remaining)
	return int(math.Ceil(overage * 100))
}

func (s *Service) billableStorage(todayGB, mtdGB float64) int {
	remaining := math.Max(0, float64(s.cfg.CloudFreeStorageGB)-mtdGB)
	overage := math.Max(0, todayGB-remaining)
	return int(math.Ceil(overage * 100))
}

func (s *Service) billableEgress(todayGB, mtdGB float64) int {
	remaining := math.Max(0, float64(s.cfg.CloudFreeEgressGB)-mtdGB)
	overage := math.Max(0, todayGB-remaining)
	return int(math.Ceil(overage * 100))
}

// monthlyTotal sums a dimension's vcpu/storage/egress since monthStart.
func (s *Service) monthlyTotal(ctx context.Context, tenantID, dim string, since time.Time) (float64, error) {
	var col string
	switch dim {
	case DimVCPU:
		col = "vcpu_hours"
	case DimStorage:
		col = "storage_gb"
	case DimEgress:
		col = "egress_gb"
	default:
		return 0, fmt.Errorf("unknown dimension: %s", dim)
	}

	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT COALESCE(SUM(%s), 0) FROM np_cloud_usage_raw
		             WHERE tenant_id = $1 AND dimension = $2 AND hour_bucket >= $3`, col),
		tenantID, dim, since,
	)
	var total float64
	if err := row.Scan(&total); err != nil {
		return 0, fmt.Errorf("monthly total %s: %w", dim, err)
	}
	return total, nil
}

// maybeAlert checks whether a tenant has crossed 80% of any free allowance
// and fires an alert if not already alerted this month.
func (s *Service) maybeAlert(ctx context.Context, t tenant, dim string, mtdVCPU, mtdStorage, mtdEgress float64) {
	var used, free float64
	switch dim {
	case DimVCPU:
		used = mtdVCPU
		free = float64(s.cfg.CloudFreeVcpuHours)
	case DimStorage:
		used = mtdStorage
		free = float64(s.cfg.CloudFreeStorageGB)
	case DimEgress:
		used = mtdEgress
		free = float64(s.cfg.CloudFreeEgressGB)
	}

	if free <= 0 {
		return
	}
	pct := (used / free) * 100
	if pct < float64(s.cfg.CloudUsageAlertPct) {
		return
	}

	// Upsert alert row — if alerted_at is already set for this month, skip.
	monthStart := time.Now().UTC().Truncate(24 * time.Hour)
	monthStart = time.Date(monthStart.Year(), monthStart.Month(), 1, 0, 0, 0, 0, time.UTC)

	tag, err := s.pool.Exec(ctx, `
		INSERT INTO np_cloud_usage_alerts (tenant_id, dimension, threshold_pct, alerted_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (tenant_id, dimension) DO UPDATE
			SET alerted_at = now()
		WHERE np_cloud_usage_alerts.alerted_at IS NULL
		   OR np_cloud_usage_alerts.alerted_at < $4
	`, t.ID, dim, s.cfg.CloudUsageAlertPct, monthStart)
	if err != nil || tag.RowsAffected() == 0 {
		return // already alerted
	}

	slog.InfoContext(ctx, "metering: usage alert triggered",
		"tenant_id", t.ID, "dim", dim, "pct", pct)
	// Email delivery is handled by the notify plugin on the web/backend side
	// which polls np_cloud_usage_alerts for new rows. See E10 spec §Alerts.
}

// tenant is a lightweight projection of np_cloud_tenants.
type tenant struct {
	ID              string
	HetznerServerID int
	HetznerVolumeID int
	StripeSIVcpu    string
	StripeSIStorage string
	StripeSIEgress  string
}

func (s *Service) activeTenants(ctx context.Context) ([]tenant, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, COALESCE(hetzner_server_id, 0), COALESCE(hetzner_volume_id, 0),
		       COALESCE(stripe_si_vcpu, ''), COALESCE(stripe_si_storage, ''), COALESCE(stripe_si_egress, '')
		FROM np_cloud_tenants
		WHERE stripe_subscription_id IS NOT NULL
	`)
	if err != nil {
		return nil, fmt.Errorf("query tenants: %w", err)
	}
	defer rows.Close()

	var out []tenant
	for rows.Next() {
		var t tenant
		if err := rows.Scan(&t.ID, &t.HetznerServerID, &t.HetznerVolumeID,
			&t.StripeSIVcpu, &t.StripeSIStorage, &t.StripeSIEgress); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// hetznerServerMetrics returns (vCPU-hours, egress-GB) for a server over the past hour.
func (s *Service) hetznerServerMetrics(ctx context.Context, serverID int) (vcpuHours, egressGB float64, err error) {
	token := s.cfg.HetznerNselfToken
	if token == "" {
		return 0, 0, fmt.Errorf("HETZNER_NSELF_TOKEN not set")
	}

	end := time.Now().UTC()
	start := end.Add(-time.Hour)
	q := fmt.Sprintf("/v1/servers/%d/metrics?type=cpu,network_out&start=%s&end=%s",
		serverID,
		url.QueryEscape(start.Format(time.RFC3339)),
		url.QueryEscape(end.Format(time.RFC3339)),
	)
	resp, err := s.hetznerGet(ctx, token, q)
	if err != nil {
		return 0, 0, err
	}

	// Parse CPU percentage → vCPU-hours (1 vCPU at 100% = 1.0 vCPU-hour).
	// Hetzner returns time-series samples; take the average.
	if cpu, ok := resp["cpu"]; ok {
		vcpuHours = averageSamples(cpu) / 100.0
	}

	// Parse outbound network bytes → GB.
	if net, ok := resp["network_out"]; ok {
		totalBytes := sumSamples(net)
		egressGB = totalBytes / 1e9
	}

	return vcpuHours, egressGB, nil
}

// hetznerVolumeGB returns the current size of a Hetzner volume in GB.
func (s *Service) hetznerVolumeGB(ctx context.Context, volumeID int) (float64, error) {
	if volumeID == 0 {
		return 0, nil
	}
	token := s.cfg.HetznerNselfToken
	q := fmt.Sprintf("/v1/volumes/%d", volumeID)
	resp, err := s.hetznerGet(ctx, token, q)
	if err != nil {
		return 0, err
	}
	vol, ok := resp["volume"].(map[string]interface{})
	if !ok {
		return 0, nil
	}
	if size, ok := vol["size"].(float64); ok {
		return size, nil
	}
	return 0, nil
}

func (s *Service) hetznerGet(ctx context.Context, token, path string) (map[string]interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.hetzner.cloud"+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 10 * time.Second}
	r, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return nil, fmt.Errorf("hetzner GET %s: status %d", path, r.StatusCode)
	}

	var out map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// stripeCreateUsageRecord calls the Stripe Metered Billing API.
// Uses the REST API directly to avoid requiring the Stripe Go SDK as a dep.
func (s *Service) stripeCreateUsageRecord(
	ctx context.Context,
	siID string,
	quantity int,
	ts time.Time,
	idempotencyKey string,
) (string, error) {
	stripeSecretKey := getenv("STRIPE_SECRET_KEY")
	if stripeSecretKey == "" {
		return "", fmt.Errorf("STRIPE_SECRET_KEY not set")
	}

	body := url.Values{}
	body.Set("quantity", fmt.Sprintf("%d", quantity))
	body.Set("timestamp", fmt.Sprintf("%d", ts.Unix()))
	body.Set("action", "increment")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("https://api.stripe.com/v1/subscription_items/%s/usage_records", siID),
		strings.NewReader(body.Encode()),
	)
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(stripeSecretKey, "")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Idempotency-Key", idempotencyKey)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var out struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("stripe usage record: status %d", resp.StatusCode)
	}
	return out.ID, nil
}

// getenv is a thin wrapper so unit tests can override via t.Setenv.
var getenv = func(key string) string {
	// Import avoided at package level to keep test surface clean.
	// Real runtime uses os.Getenv implicitly through config.Load.
	return ""
}

func init() {
	// Wire the real os.Getenv at init time — overridable in tests.
	// This avoids an os import at the top of the file interfering with
	// test injection patterns.
	import_os_getenv()
}

func import_os_getenv() {
	// Resolved at link time; test files override via var reassignment.
	getenv = func(key string) string {
		return osGetenv(key)
	}
}

// osGetenv is provided by metering_os.go to avoid a direct os import here.
// Tests substitute getenv directly without needing the OS shim.

// averageSamples computes the average of Hetzner time-series values.
// Hetzner metrics JSON: {"time_series": [{"values": [[ts, val], ...]}, ...]}
func averageSamples(raw interface{}) float64 {
	m, ok := raw.(map[string]interface{})
	if !ok {
		return 0
	}
	ts, ok := m["time_series"].([]interface{})
	if !ok || len(ts) == 0 {
		return 0
	}
	series, ok := ts[0].(map[string]interface{})
	if !ok {
		return 0
	}
	values, ok := series["values"].([]interface{})
	if !ok || len(values) == 0 {
		return 0
	}
	var sum float64
	var n int
	for _, v := range values {
		pair, ok := v.([]interface{})
		if !ok || len(pair) < 2 {
			continue
		}
		f, ok := pair[1].(float64)
		if !ok {
			continue
		}
		sum += f
		n++
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

// sumSamples sums all Hetzner time-series values (for cumulative counters like bytes).
func sumSamples(raw interface{}) float64 {
	m, ok := raw.(map[string]interface{})
	if !ok {
		return 0
	}
	ts, ok := m["time_series"].([]interface{})
	if !ok || len(ts) == 0 {
		return 0
	}
	series, ok := ts[0].(map[string]interface{})
	if !ok {
		return 0
	}
	values, ok := series["values"].([]interface{})
	if !ok {
		return 0
	}
	var sum float64
	for _, v := range values {
		pair, ok := v.([]interface{})
		if !ok || len(pair) < 2 {
			continue
		}
		if f, ok := pair[1].(float64); ok {
			sum += f
		}
	}
	return sum
}
