// Command bootmig wires the boot-time migration helper into a Go plugin (contract plugin.boot-migrations v1).
//
// Usage: bootmig [-dry] -dir <plugin dir> -schema np_<name>
// It finds the one pgxpool.New call of the plugin, inserts `mustBootMigrations(ctx, pool)` after its error
// check, writes boot_migrations.go next to it and migrations/embed.go (go:embed *.sql), and adds the
// "migrations" member to the /health map literal. Edits are text insertions at AST offsets, so comments and
// layout survive; each edited file goes through gofmt. A pool pattern it does not find is reported as
// "manual: <why>" and nothing is written. Prints "noop", "manual: <why>" or "converted <details>".
// Idempotent: a plugin that already calls mustBootMigrations or migrate.Apply prints "noop".
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	dir := flag.String("dir", "", "plugin directory")
	schema := flag.String("schema", "", "plugin schema np_<name>")
	dry := flag.Bool("dry", false, "report only, write nothing")
	flag.Parse()
	if *dir == "" || *schema == "" {
		fmt.Fprintln(os.Stderr, "usage: bootmig [-dry] -dir <plugin dir> -schema np_<name>")
		os.Exit(2)
	}
	out, err := run(*dir, *schema, *dry)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(out)
}
