package economics

import (
	"context"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// ReplaceUsage replaces one provider's complete report range in a single transaction.
func ReplaceUsage(ctx context.Context, provider, from, to string, lines []Usage) error {
	env, ok := envOf(ctx)
	if !ok {
		return refuse(model.AllowanceUnknown, "economics is not bound, so usage cannot be recorded")
	}
	rows := make([]store.UsageRow, 0, len(lines))
	for _, u := range lines {
		row, err := usageRow(env, u)
		if err != nil {
			return err
		}
		rows = append(rows, row)
	}
	return env.Store.ReplaceUsage(ctx, provider, from, to, rows)
}
