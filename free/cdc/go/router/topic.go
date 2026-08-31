// Package router maps Postgres table names and operations to broker topic strings.
package router

import "strings"

// TopicRouter converts (table, operation) pairs into topic names.
type TopicRouter struct {
	prefix string
}

// New creates a TopicRouter with the given prefix (e.g. "nself").
func New(prefix string) *TopicRouter {
	if prefix == "" {
		prefix = "nself"
	}
	return &TopicRouter{prefix: prefix}
}

// Topic returns the canonical topic string for a table and operation.
// Format: <prefix>.<table>.<operation_lowercase>
// Examples:
//   - nself.np_users.insert
//   - nself.np_orders.delete
func (r *TopicRouter) Topic(table, operation string) string {
	return r.prefix + "." + table + "." + strings.ToLower(operation)
}

// Prefix returns the configured topic prefix.
func (r *TopicRouter) Prefix() string { return r.prefix }
