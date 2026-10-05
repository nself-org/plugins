package main

// Purpose: the "forgejo" subcommand: Forgejo server and runner health, ported
// from core `nself ci forgejo` (P7-CANON-09). The implementation lives in
// internal/forgejo; this file parses the flags only.
// Inputs:  --url (default http://localhost:3844), --runner.
// Outputs: status table on stdout; exit 0 (core never failed this command).
// Constraints: output and exit codes identical to core on fixtures.

import (
	"flag"

	"github.com/nself-org/plugins/free/ci/internal/forgejo"
)

func init() {
	register("forgejo", func(args []string) int { return forgejoCmd(args, flag.ExitOnError) })
}

// forgejoCmd parses the flags and runs the check.
func forgejoCmd(args []string, eh flag.ErrorHandling) int {
	var cfg forgejo.Config
	fs := flag.NewFlagSet("forgejo", eh)
	fs.StringVar(&cfg.URL, "url", forgejo.DefaultURL, "Forgejo base URL")
	fs.StringVar(&cfg.Runner, "runner", "", "Runner container name (default: <PROJECT>_forgejo_runner)")
	if _, err := parseInterspersed(fs, args); err != nil {
		return 2
	}
	if err := forgejo.Run(cfg); err != nil {
		return 1
	}
	return 0
}
