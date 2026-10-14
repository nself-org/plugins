package main

// Purpose: the "config show" and "config migrate" subcommands of nself-ci
// (P7-CI-36, Epic D7). The logic lives in internal/config; this file
// registers the two keys, V15 only, so v1.4 mode is unchanged.
// Inputs:  the flags of each command (see internal/config/cli.go).
// Outputs: text or one v1 JSON envelope on stdout; exit 0, 1 (invalid
// config, leftover .ci.yaml) or 2 (infrastructure).
// Constraints: cmd/main.go is untouched; registration happens from init().

import (
	"os"

	"github.com/nself-org/plugins/free/ci/internal/config"
)

func init() {
	registerV15("config show", subcommandEntry{
		V15Only: true,
		Flags:   config.ShowFlags,
		Handler: func(args []string) int {
			return config.RunShow(args, os.Stdout, os.Stderr, config.DefaultDeps())
		},
	})
	registerV15("config migrate", subcommandEntry{
		V15Only: true,
		Flags:   config.MigrateFlags,
		Handler: func(args []string) int {
			return config.RunMigrate(args, os.Stdout, os.Stderr, config.DefaultDeps())
		},
	})
}
