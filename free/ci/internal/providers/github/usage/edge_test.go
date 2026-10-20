package usage_test

// Edge tests of the importer and source: periods that cross a month, errors that mean "try again"
// (not E709), malformed amounts, and the daily Reconciler.

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/usage"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func TestPeriodAcrossMonthsAsksOncePerMonth(t *testing.T) {
	f := newFakeBilling(t, map[string]reply{orgPath: {status: 200, file: "org_october.json"}})
	src := f.source(t, "org:acme")
	day := func(m time.Month, d int) time.Time { return time.Date(2026, m, d, 0, 0, 0, 0, time.UTC) }
	rows, err := src.Usage(context.Background(), providers.Period{From: day(9, 28), To: day(10, 20)})
	if err != nil || f.callCount() != 2 {
		t.Fatalf("calls=%d err=%v", f.callCount(), err)
	}
	if f.calls[0] != orgPath+"?month=9&year=2026" || f.calls[1] != orgPath+"?month=10&year=2026" {
		t.Fatalf("queries: %v", f.calls)
	}
	if len(rows) != 8 { // the same fixture answers both months: its 4 Actions minute lines are all in the window
		t.Fatalf("rows: %d", len(rows))
	}
	// A one-day period adds the day query.
	f2 := newFakeBilling(t, map[string]reply{orgPath: {status: 200, file: "org_october.json"}})
	rows, err = f2.source(t, "org:acme").Usage(context.Background(), providers.Period{From: day(10, 14), To: day(10, 14)})
	if err != nil || len(rows) != 2 || f2.calls[0] != orgPath+"?day=14&month=10&year=2026" {
		t.Fatalf("one day: %d rows %v %v", len(rows), f2.calls, err)
	}
	if _, err = src.Usage(context.Background(), providers.Period{From: day(1, 1), To: time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC)}); err == nil {
		t.Fatal("a period over MaxMonths must be refused")
	}
}

func TestRetryableErrorsAreNotUnavailable(t *testing.T) {
	for name, rep := range map[string]reply{
		"rate limited": {status: 403, header: map[string]string{"X-RateLimit-Remaining": "0"}},
		"too many":     {status: 429, header: map[string]string{"Retry-After": "30"}},
		"server error": {status: 502},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFakeBilling(t, map[string]reply{orgPath: rep})
			rec := &recorder{}
			_, err := importer(f.source(t, "org:acme"), "org:acme", true, rec).Run(ledgerCtx(openLedger(t)))
			var pe *providers.Error
			if !errors.As(err, &pe) || pe.Code == "E709" || len(rec.keys()) != 0 {
				t.Fatalf("a retry hint must stay a retry hint, nothing written: %v", err)
			}
		})
	}
}

func TestMissingCredentialStaysE700(t *testing.T) {
	f := newFakeBilling(t, map[string]reply{orgPath: {status: 200, file: "org_october.json"}})
	c := f.client()
	c.Auth = func(context.Context) (string, error) {
		return "", providers.NewError(providers.Config, "E700", "env:X", 0, time.Time{})
	}
	acct, _ := usage.ParseAccount("org:acme")
	src, _ := usage.New(usage.Config{Client: c, Account: acct})
	_, err := importer(src, "org:acme", true, &recorder{}).Run(context.Background())
	var pe *providers.Error
	if !errors.As(err, &pe) || pe.Code != "E700" {
		t.Fatalf("got %v", err)
	}
}

func TestMalformedAmountsWriteNothing(t *testing.T) {
	for name, line := range map[string]string{
		"negative":  `{"date":"2026-10-14T00:00:00Z","product":"Actions","sku":"Actions Linux","quantity":-5,"unitType":"minutes","netAmount":1,"repositoryName":"a/b"}`,
		"no amount": `{"date":"2026-10-14T00:00:00Z","product":"Actions","sku":"Actions Linux","quantity":5,"unitType":"minutes","repositoryName":"a/b"}`,
		"bad date":  `{"date":"yesterday","product":"Actions","sku":"Actions Linux","quantity":5,"unitType":"minutes","netAmount":1,"repositoryName":"a/b"}`,
		"no sku":    `{"date":"2026-10-14T00:00:00Z","product":"Actions","quantity":5,"unitType":"minutes","netAmount":1,"repositoryName":"a/b"}`,
	} {
		t.Run(name, func(t *testing.T) {
			f := newFakeBilling(t, map[string]reply{orgPath: {status: 200, body: `{"usageItems":[` + line + `]}`}})
			rec := &recorder{}
			_, err := importer(f.source(t, "org:acme"), "org:acme", true, rec).Run(ledgerCtx(openLedger(t)))
			var pe *providers.Error
			if !errors.As(err, &pe) || pe.Code != "E709" || len(rec.keys()) != 0 {
				t.Fatalf("want E709 and no rows, got %v / %d", err, len(rec.keys()))
			}
		})
	}
}

func TestEnabledWithoutAccountIsE700(t *testing.T) {
	_, err := importer(tripwire{t}, "", true, &recorder{}).Run(context.Background())
	var pe *providers.Error
	if !errors.As(err, &pe) || pe.Code != "E700" {
		t.Fatalf("got %v", err)
	}
}

// ---- Reconciler ----

type marks struct {
	mu sync.Mutex
	m  map[string]time.Time
}

func (m *marks) ProviderMarkGet(_ context.Context, name, key string) (time.Time, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	at, ok := m.m[name+"|"+key]
	return at, ok, nil
}

func (m *marks) ProviderMarkPut(_ context.Context, name, key string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.m[name+"|"+key] = at
	return nil
}

func operatorPolicy(t *testing.T, kv map[string]string) *policy.Effective {
	t.Helper()
	eff, err := merge(t, trust.SourceUser, kv)
	if err != nil {
		t.Fatal(err)
	}
	return &eff
}

func TestReconcilerImportsOncePerDay(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	eff := operatorPolicy(t, map[string]string{usage.KeyBillingImport: `true`, usage.KeyBillingAccount: `"org:acme"`})
	f := newFakeBilling(t, map[string]reply{orgPath: {status: 200, file: "org_october.json"}})
	rec := &recorder{}
	mk := &marks{m: map[string]time.Time{}}
	now := today
	r := &usage.Reconciler{Marks: mk, Clock: func() time.Time { return now },
		Policy: func(context.Context) (*policy.Effective, error) { return eff, nil },
		Build: func(context.Context, *policy.Effective) (*usage.Importer, error) {
			return importer(f.source(t, "org:acme"), "org:acme", true, rec), nil
		}}
	var _ coordinator.Reconciler = r
	st := openLedger(t)
	r.Store = st
	ctx := context.Background()
	if err := r.Reconcile(ctx, nil); err != nil || f.callCount() != 1 || len(rec.keys()) != 2 {
		t.Fatalf("first pass: calls=%d keys=%d err=%v", f.callCount(), len(rec.keys()), err)
	}
	now = today.Add(5 * time.Hour)
	if err := r.Reconcile(ctx, nil); err != nil || f.callCount() != 1 {
		t.Fatalf("a pass inside the interval must not call the API: calls=%d err=%v", f.callCount(), err)
	}
	now = today.Add(usage.Interval + time.Minute)
	if err := r.Reconcile(ctx, nil); err != nil || f.callCount() != 2 {
		t.Fatalf("a pass after the interval imports again: calls=%d err=%v", f.callCount(), err)
	}
	if r.Name() != usage.Name || r.Every() != usage.Every {
		t.Fatal("name or cadence")
	}
}

func TestReconcilerFailureKeepsTheMarkOpen(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	eff := operatorPolicy(t, map[string]string{usage.KeyBillingImport: `true`, usage.KeyBillingAccount: `"org:acme"`})
	f := newFakeBilling(t, map[string]reply{orgPath: {status: 403, file: "forbidden.json"}})
	mk := &marks{m: map[string]time.Time{}}
	r := &usage.Reconciler{Marks: mk, Clock: func() time.Time { return today }, Store: openLedger(t),
		Policy: func(context.Context) (*policy.Effective, error) { return eff, nil },
		Build: func(context.Context, *policy.Effective) (*usage.Importer, error) {
			return importer(f.source(t, "org:acme"), "org:acme", true, &recorder{}), nil
		}}
	if err := r.Reconcile(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "E709") {
		t.Fatalf("got %v", err)
	}
	if len(mk.m) != 0 {
		t.Fatalf("a failed import moved the mark: %v", mk.m)
	}
}

func TestReconcilerDormantAndOff(t *testing.T) {
	built := false
	build := func(context.Context, *policy.Effective) (*usage.Importer, error) {
		built = true
		return nil, errors.New("must not build")
	}
	mk := &marks{m: map[string]time.Time{}}
	on := operatorPolicy(t, map[string]string{usage.KeyBillingImport: `true`, usage.KeyBillingAccount: `"org:acme"`})
	off := operatorPolicy(t, map[string]string{usage.KeyBillingImport: `false`})
	cases := map[string]struct {
		v15 string
		eff *policy.Effective
	}{"no V15": {"", on}, "import off": {"1", off}, "unset": {"1", operatorPolicy(t, nil)}}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv("NSELF_V15", c.v15)
			r := &usage.Reconciler{Marks: mk, Build: build, Policy: func(context.Context) (*policy.Effective, error) { return c.eff, nil }}
			if err := r.Reconcile(context.Background(), nil); err != nil || built || len(mk.m) != 0 {
				t.Fatalf("err=%v built=%v marks=%v", err, built, mk.m)
			}
		})
	}
	t.Setenv("NSELF_V15", "1")
	if err := (&usage.Reconciler{}).Reconcile(context.Background(), nil); err != nil {
		t.Fatalf("no policy installed yet: %v", err)
	}
}
