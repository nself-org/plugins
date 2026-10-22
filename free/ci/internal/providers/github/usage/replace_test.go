package usage_test

import (
	"context"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/usage"
)

type usageRows []providers.UsageRow

func (r usageRows) Usage(context.Context, providers.Period) ([]providers.UsageRow, error) {
	return r, nil
}

func TestImporterReplacesRenamedAndRemovedRows(t *testing.T) {
	for _, change := range []string{"repo", "sku", "account", "removed"} {
		t.Run(change, func(t *testing.T) {
			st := openLedger(t)
			ctx := ledgerCtx(st)
			row := providers.UsageRow{Date: today, SKU: "Linux", Repo: "old/repo", Quantity: 10, NetMicro: 100}
			imp := &usage.Importer{Settings: usage.Settings{Import: true, Account: "org:acme"}, Source: usageRows{row}, Now: func() time.Time { return today }}
			if _, err := imp.Run(ctx); err != nil {
				t.Fatal(err)
			}
			switch change {
			case "repo":
				row.Repo = "new/repo"
			case "sku":
				row.SKU = "macOS"
			case "account":
				imp.Settings.Account = "org:other"
			}
			imp.Source = usageRows{row}
			want := int64(100)
			if change == "removed" {
				imp.Source = usageRows{}
				want = 0
			}
			if _, err := imp.Run(ctx); err != nil {
				t.Fatal(err)
			}
			got, err := st.Allowance(ctx, usage.Provider, "2026-10")
			if err != nil || got.Imported != want {
				t.Fatalf("imported=%d want=%d err=%v", got.Imported, want, err)
			}
		})
	}
}
