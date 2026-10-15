// Package migrations embeds the plugin's SQL migration files for sdk/go/migrate.
package migrations

import "embed"

// FS holds migrations/*.sql (the *.down.sql files are not applied by the loader).
//
//go:embed *.sql
var FS embed.FS
