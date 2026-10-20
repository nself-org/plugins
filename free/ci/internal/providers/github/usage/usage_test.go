package usage_test

// Tests of the GitHub enhanced-billing importer (P7-HOST-10, Epic D12). Each acceptance test names the
// acceptance line it proves in its doc comment. Fixtures are synthetic and a fake TLS server stands
// in for GitHub; the server fails the test on any request outside the one allowed endpoint.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/economics"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/usage"
)

const (
	orgPath  = "/organizations/acme/settings/billing/usage"
	userPath = "/users/octo/settings/billing/usage"
)

func importer(src providers.UsageSource, account string, on bool, rec *recorder) *usage.Importer {
	return &usage.Importer{Settings: usage.Settings{Import: on, Account: account}, Source: src, Record: rec.record, Now: func() time.Time { return today }}
}

// TestDisabled proves acceptance 1: with billing_import false the command prints `disabled` and exits
// 0 without an API call. The importer returns Disabled before it touches the source (a source that
// fails the test if called proves no request or credential read), and an unset policy builds no client.
func TestDisabled(t *testing.T) {
	rec := &recorder{}
	imp := importer(tripwire{t}, "org:acme", false, rec)
	res, err := imp.Run(context.Background())
	if err != nil || !res.Disabled || res.Rows != 0 || len(rec.keys()) != 0 {
		t.Fatalf("disabled import: %+v, %v, %d rows written", res, err, len(rec.keys()))
	}
	// No source at all is fine while the import is off, and the policy path builds no client.
	if res, err = (&usage.Importer{Settings: usage.DefaultSettings()}).Run(context.Background()); err != nil || !res.Disabled {
		t.Fatalf("default settings must be disabled: %+v, %v", res, err)
	}
	var eff policy.Effective
	fromPolicy, err := usage.FromPolicy(context.Background(), &eff)
	if err != nil || fromPolicy.Source != nil {
		t.Fatalf("an unset policy must build a disabled importer without a source: %+v, %v", fromPolicy, err)
	}
	if res, err = fromPolicy.Run(context.Background()); err != nil || !res.Disabled {
		t.Fatalf("unset policy: %+v, %v", res, err)
	}
}

type tripwire struct{ t *testing.T }

func (w tripwire) Usage(context.Context, providers.Period) ([]providers.UsageRow, error) {
	w.t.Error("the source was called although billing_import is false")
	return nil, nil
}

// TestImportRows proves acceptance 2: with user and org fixtures, Actions minute rows are written
// through RecordUsage with confidence known, other products are ignored, and the `ci policy usage`
// report shows them as provider-reported (known) usage.
func TestImportRows(t *testing.T) {
	t.Run("org", func(t *testing.T) {
		f := newFakeBilling(t, map[string]reply{orgPath: {status: 200, file: "org_october.json"}})
		st := openLedger(t)
		rec := &recorder{}
		res, err := importer(f.source(t, "org:acme"), "org:acme", true, rec).Run(ledgerCtx(st))
		if err != nil || res.Rows != 2 || res.Account != "org:acme" || res.From != "2026-10-13" || res.To != "2026-10-19" {
			t.Fatalf("result: %+v, %v", res, err)
		}
		keys := rec.keys()
		linux := keys[usage.Key("org:acme", "2026-10-14", "Actions Linux", "acme/widgets")]
		mac := keys[usage.Key("org:acme", "2026-10-15", "Actions macOS 3-core", "acme/gadgets")]
		if len(keys) != 2 || linux.Minutes != 150 || linux.CostMicro != 240_000 || mac.Minutes != 10 || mac.CostMicro != 620_000 {
			t.Fatalf("rows (storage, Copilot, Packages and the out-of-window day must be absent; same-day duplicates fold): %+v", keys)
		}
		if linux.Provider != usage.Provider || linux.Series != "Actions Linux" || linux.Date != "2026-10-14" {
			t.Fatalf("line fields: %+v", linux)
		}
		if got := f.calls[0]; got != orgPath+"?month=10&year=2026" {
			t.Fatalf("query: %q", got)
		}
		line := githubLine(t, st)
		if line.Imported == nil || line.Imported.Micro != 860_000 || line.Imported.Confidence != economics.ConfKnown {
			t.Fatalf("the ledger report must show the import as known: %+v", line.Imported)
		}
	})
	t.Run("user", func(t *testing.T) {
		f := newFakeBilling(t, map[string]reply{userPath: {status: 200, file: "user_october.json"}})
		st := openLedger(t)
		rec := &recorder{}
		res, err := importer(f.source(t, "user:octo"), "user:octo", true, rec).Run(ledgerCtx(st))
		if err != nil || res.Rows != 1 {
			t.Fatalf("result: %+v, %v", res, err)
		}
		l := rec.keys()[usage.Key("user:octo", "2026-10-17", "Actions Linux", "octo/hello")]
		if l.Minutes != 45 || l.CostMicro != 0 {
			t.Fatalf("a fully discounted line is a known zero, not a missing row: %+v", l)
		}
		if line := githubLine(t, st); line.Imported == nil || line.Imported.Micro != 0 || line.Imported.Confidence != economics.ConfKnown {
			t.Fatalf("report: %+v", line.Imported)
		}
	})
}

// TestIdempotent proves acceptance 3: re-importing the same period writes no duplicate rows. The
// second run produces the same keys and the report total does not move.
func TestIdempotent(t *testing.T) {
	f := newFakeBilling(t, map[string]reply{orgPath: {status: 200, file: "org_october.json"}})
	st := openLedger(t)
	first, second := &recorder{}, &recorder{}
	src := f.source(t, "org:acme")
	if _, err := importer(src, "org:acme", true, first).Run(ledgerCtx(st)); err != nil {
		t.Fatal(err)
	}
	before := githubLine(t, st).Imported.Micro
	if _, err := importer(src, "org:acme", true, second).Run(ledgerCtx(st)); err != nil {
		t.Fatal(err)
	}
	a, b := first.keys(), second.keys()
	if len(a) != 2 || len(b) != 2 {
		t.Fatalf("keys: %v / %v", a, b)
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			t.Fatalf("second import wrote a different key %q", k)
		}
	}
	if after := githubLine(t, st).Imported.Micro; after != before || before != 860_000 {
		t.Fatalf("the total moved from %d to %d: a duplicate row was written", before, after)
	}
}

// TestUnavailable proves acceptance 4: 403, 404 and 410 fixtures (and a body without usageItems, the
// shape of a changed preview) yield E709 with the reason and write nothing; the summary and retired
// endpoints are never requested (the fixture server fails the test on them).
func TestUnavailable(t *testing.T) {
	cases := []struct {
		name   string
		reply  reply
		reason string
	}{
		{"forbidden", reply{status: 403, file: "forbidden.json"}, "http_403"},
		{"not found", reply{status: 404, file: "not_found.json"}, "http_404"},
		{"gone", reply{status: 410, file: "gone.json"}, "http_410"},
		{"preview shape changed", reply{status: 200, file: "preview_changed.json"}, "shape.usageItems"},
		{"empty body", reply{status: 200}, "shape.usageItems"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFakeBilling(t, map[string]reply{orgPath: c.reply})
			st := openLedger(t)
			rec := &recorder{}
			_, err := importer(f.source(t, "org:acme"), "org:acme", true, rec).Run(ledgerCtx(st))
			var pe *providers.Error
			if !errors.As(err, &pe) || pe.Code != "E709" || pe.Reason != c.reason {
				t.Fatalf("want E709 %s, got %v", c.reason, err)
			}
			if n := len(rec.keys()); n != 0 {
				t.Fatalf("%d rows were written for an unavailable report", n)
			}
			if rep, err := economics.BuildReport(context.Background(), st, economicsPolicy(), today); err != nil || len(rep.Providers) != 0 {
				t.Fatalf("the ledger must stay empty: %+v, %v", rep.Providers, err)
			}
		})
	}
}
