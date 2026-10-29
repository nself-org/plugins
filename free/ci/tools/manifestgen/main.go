package main

// Purpose: `go generate` writer of the plugin.json `commands` block from surface/commands.yaml (P7-CI-62, CI D31).
// Inputs:  --spec PATH (default ../../surface/commands.yaml), --manifest PATH (default ../../plugin.json), --check.
// Outputs: rewrites the manifest in place; with --check it writes nothing and exits 1 when the manifest differs.
// Constraints: touches only the `commands` key; exit 0 ok, 1 stale or invalid spec, 2 usage or I/O error.

//go:generate go run . --spec ../../surface/commands.yaml --manifest ../../plugin.json

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("manifestgen", flag.ContinueOnError)
	fs.SetOutput(stderr)
	specPath := fs.String("spec", "../../surface/commands.yaml", "the authored machine-surface spec")
	manifest := fs.String("manifest", "../../plugin.json", "the plugin manifest to update")
	check := fs.Bool("check", false, "report a stale manifest instead of writing")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	raw, err := os.ReadFile(*specPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	cur, err := os.ReadFile(*manifest)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	spec, err := Parse(raw)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	next, err := Render(spec, cur)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if bytes.Equal(cur, next) {
		return 0
	}
	if *check {
		fmt.Fprintln(stderr, "plugin.json commands is stale: run go generate ./tools/manifestgen/...")
		return 1
	}
	if err := os.WriteFile(*manifest, next, 0o644); err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	fmt.Fprintln(stdout, "wrote", *manifest)
	return 0
}
