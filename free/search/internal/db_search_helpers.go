package internal

import (
	"context"
	"fmt"
)

// suitable for to_tsvector.
func buildSearchText(searchableFields []string, fields map[string]interface{}) string {
	var text string
	for _, f := range searchableFields {
		v, ok := fields[f]
		if !ok || v == nil {
			continue
		}
		s := fmt.Sprintf("%v", v)
		if text != "" {
			text += " "
		}
		text += s
	}
	return text
}

// sourceAccountKey is the context key for multi-app isolation.
type sourceAccountKey struct{}

// WithSourceAccount returns a context carrying the given source_account_id.
func WithSourceAccount(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, sourceAccountKey{}, id)
}

// resolveSourceAccountID extracts the source_account_id from context or defaults to "primary".
func resolveSourceAccountID(ctx context.Context) string {
	if v, ok := ctx.Value(sourceAccountKey{}).(string); ok && v != "" {
		return v
	}
	return "primary"
}
