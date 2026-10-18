// nself-eval — the `nself eval` command of the nself-eval-gate plugin.
//
// Purpose: run an eval suite (or validate an eval-set YAML) against the
//
//	nself-eval-gate service, and check an autonomy tier gate. It is the
//	port of core `nself ci eval [gate]` (P7-CI-44), so `nself eval …` and
//	`nself-eval …` take the same argv.
//
// Usage:
//
//	nself-eval --suite <slug> [--output json|table|summary] [--repo DIR]
//	nself-eval --validate-only --suite <slug>
//	nself-eval gate --tier supervised|semi-auto|full-auto
//
// Exit codes: 0 passed/cleared, 1 below threshold/blocked, 2 validation or
// infra error, 3 precondition not met (BGE-M3 or AI gateway down).
//
// SPORT: PLUGINS-NSELF-EVAL-GATE-CLI-001
package main

import (
	"os"

	"github.com/nself-org/nself-eval-gate/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
