package usage_test

// Tests of the period replacement of the importer (D-1317): consumption GitHub reports under a new repo
// name, sku or billing account must not be counted next to the rows the old names left behind.

import (
	"context"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/usage"
)

// stubSource answers every request with its rows.
type stubSource struct{ rows []providers.UsageRow }

func (s *stubSource) Usage(context.Context, providers.Period) ([]providers.UsageRow, error) {
	return s.rows, nil
}

func row(account, repo, sku string, micro int64) providers.UsageRow {
	return providers.UsageRow{Account: account, Date: time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC), Product: "Actions", SKU: sku,
		Quantity: 10, Unit: "minutes", NetMicro: micro, Repo: repo}
}

func TestImportReplacesRowsUnderOldNames(t *testing.T) {
	st := openLedger(t)
	ctx := ledgerCtx(st)
	run := func(account string, rows ...providers.UsageRow) {
		t.Helper()
		if _, err := importer(&stubSource{rows}, account, true, &recorder{}).Run(ctx); err != nil {
			t.Fatal(err)
		}
	}
	total := func() int64 {
		t.Helper()
		in, err := st.Allowance(context.Background(), usage.Provider, "2026-10")
		if err != nil {
			t.Fatal(err)
		}
		return in.Imported
	}
	run("org:acme", row("org:acme", "acme/old", "Actions Linux", 500_000))
	if got := total(); got != 500_000 {
		t.Fatalf("first import counts %d, want 500000", got)
	}
	run("org:acme", row("org:acme", "acme/new", "Actions Linux", 500_000)) // the repo was renamed
	if got := total(); got != 500_000 {
		t.Fatalf("after a repo rename the allowance counts %d, want the same 500000 once", got)
	}
	run("org:acme", row("org:acme", "acme/new", "Actions Linux 2-core", 500_000)) // the sku was renamed
	if got := total(); got != 500_000 {
		t.Fatalf("after a sku rename the allowance counts %d, want 500000 once", got)
	}
	run("org:other", row("org:other", "acme/new", "Actions Linux 2-core", 500_000)) // billing_account changed
	if got := total(); got != 500_000 {
		t.Fatalf("after a billing account change the allowance counts %d, want 500000 once", got)
	}
}

// An empty report is not a statement that nothing was used: it deletes nothing.
func TestEmptyReportKeepsKnownUsage(t *testing.T) {
	st := openLedger(t)
	ctx := ledgerCtx(st)
	if _, err := importer(&stubSource{[]providers.UsageRow{row("org:acme", "acme/r", "Actions Linux", 700_000)}}, "org:acme", true, &recorder{}).Run(ctx); err != nil {
		t.Fatal(err)
	}
	res, err := importer(&stubSource{}, "org:acme", true, &recorder{}).Run(ctx)
	if err != nil || res.Rows != 0 {
		t.Fatalf("empty report: %+v %v", res, err)
	}
	if in, _ := st.Allowance(context.Background(), usage.Provider, "2026-10"); in.Imported != 700_000 {
		t.Fatalf("an empty report changed the imported figure to %d", in.Imported)
	}
}
