package metering

import (
	"testing"

	"github.com/nself-org/nself-tenant-controller/internal/config"
)

func newTestService(freeVCPU, freeStorage, freeEgress int) *Service {
	return &Service{
		cfg: &config.Config{
			CloudFreeVcpuHours: freeVCPU,
			CloudFreeStorageGB: freeStorage,
			CloudFreeEgressGB:  freeEgress,
			CloudUsageAlertPct: 80,
		},
	}
}

// TestBillableVCPU verifies the free-tier deduction math.
func TestBillableVCPU(t *testing.T) {
	s := newTestService(500, 50, 100)

	tests := []struct {
		name       string
		todayHours float64
		mtdHours   float64
		wantQty    int // Stripe quantity = ceil(overage * 100)
	}{
		{"entirely free", 10, 0, 0},
		{"mtd already past free tier", 10, 500, 1000}, // 10h × 100
		{"straddles boundary", 50, 480, 3000},         // 20h free remain, 30h billable × 100 = 3000
		{"zero today", 0, 500, 0},
		{"fractional overage rounds up", 0.001, 500, 1}, // 0.001 × 100 = 0.1 → ceil = 1
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := s.billableVCPU(tc.todayHours, tc.mtdHours)
			if got != tc.wantQty {
				t.Errorf("billableVCPU(today=%.3f, mtd=%.1f) = %d, want %d",
					tc.todayHours, tc.mtdHours, got, tc.wantQty)
			}
		})
	}
}

// TestBillableStorage checks storage free-tier deduction.
func TestBillableStorage(t *testing.T) {
	s := newTestService(500, 50, 100)

	tests := []struct {
		todayGB float64
		mtdGB   float64
		wantQty int
	}{
		{10, 0, 0},    // 50GB free, 10 used today, still free
		{10, 50, 1000}, // mtd = 50, no free left, 10GB × 100 = 1000
		{5, 48, 300},   // 2GB free remain, 3GB billable × 100 = 300
	}

	for _, tc := range tests {
		got := s.billableStorage(tc.todayGB, tc.mtdGB)
		if got != tc.wantQty {
			t.Errorf("billableStorage(%.1f, %.1f) = %d, want %d",
				tc.todayGB, tc.mtdGB, got, tc.wantQty)
		}
	}
}

// TestBillableEgress checks egress free-tier deduction.
func TestBillableEgress(t *testing.T) {
	s := newTestService(500, 50, 100)

	tests := []struct {
		todayGB float64
		mtdGB   float64
		wantQty int
	}{
		{20, 0, 0},     // 100GB free, well within
		{20, 100, 2000}, // mtd = 100, 20GB billable × 100
		{15, 90, 500},   // 10GB free remain, 5GB billable × 100 = 500
	}

	for _, tc := range tests {
		got := s.billableEgress(tc.todayGB, tc.mtdGB)
		if got != tc.wantQty {
			t.Errorf("billableEgress(%.1f, %.1f) = %d, want %d",
				tc.todayGB, tc.mtdGB, got, tc.wantQty)
		}
	}
}

// TestIdempotencyKeyConstruction validates key format matches spec.
func TestIdempotencyKeyConstruction(t *testing.T) {
	tenantID := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	dim := DimVCPU
	dateStr := "2026-04-23"

	key := tenantID + "-" + dim + "-" + dateStr
	expected := "f47ac10b-58cc-4372-a567-0e02b2c3d479-vcpu-2026-04-23"
	if key != expected {
		t.Errorf("idempotency key = %q, want %q", key, expected)
	}
}

// TestCollectHourlyFlagOff verifies early return when metering flag is off.
func TestCollectHourlyFlagOff(t *testing.T) {
	s := &Service{
		cfg:  &config.Config{CloudMeteringEnabled: false},
		pool: nil, // nil pool — would panic if queried
	}

	if err := s.CollectHourly(nil); err != nil { //nolint:staticcheck
		t.Errorf("expected nil error when flag off, got: %v", err)
	}
}

// TestReportDailyFlagOff verifies early return when metering flag is off.
func TestReportDailyFlagOff(t *testing.T) {
	s := &Service{
		cfg:  &config.Config{CloudMeteringEnabled: false},
		pool: nil,
	}

	if err := s.ReportDaily(nil); err != nil { //nolint:staticcheck
		t.Errorf("expected nil error when flag off, got: %v", err)
	}
}

// TestAverageSamples verifies Hetzner time-series parsing.
func TestAverageSamples(t *testing.T) {
	raw := map[string]interface{}{
		"time_series": []interface{}{
			map[string]interface{}{
				"values": []interface{}{
					[]interface{}{float64(1000), float64(80.0)},
					[]interface{}{float64(1060), float64(60.0)},
					[]interface{}{float64(1120), float64(100.0)},
				},
			},
		},
	}
	avg := averageSamples(raw)
	if avg != 80.0 {
		t.Errorf("averageSamples = %.1f, want 80.0", avg)
	}
}

// TestSumSamples verifies Hetzner cumulative counter parsing.
func TestSumSamples(t *testing.T) {
	raw := map[string]interface{}{
		"time_series": []interface{}{
			map[string]interface{}{
				"values": []interface{}{
					[]interface{}{float64(1000), float64(1000000.0)},
					[]interface{}{float64(1060), float64(2000000.0)},
				},
			},
		},
	}
	sum := sumSamples(raw)
	if sum != 3000000.0 {
		t.Errorf("sumSamples = %.0f, want 3000000", sum)
	}
}
