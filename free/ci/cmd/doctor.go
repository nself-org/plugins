package main

// Purpose: the "doctor" subcommand of nself-ci (P7-CI-50, X18, R-DX-09). The
// logic lives in internal/doctor; this file registers the key, V15 only, so
// v1.4 mode is unchanged.
// Inputs:  --json, --nodes, --providers, --repo (see internal/doctor/cli.go).
// Outputs: a table or one v1 JSON envelope on stdout; exit 0 clean, 10 a check
// failed, 12 warnings only, 1 usage, 2 infrastructure.
// Constraints: cmd/main.go is untouched; registration happens from init().

import (
	"os"

	"github.com/nself-org/plugins/free/ci/internal/doctor"
)

func init() {
	registerV15("doctor", subcommandEntry{
		V15Only: true, SideEffect: SideEffectRead,
		Flags: doctor.Flags,
		Handler: func(args []string) int {
			return doctor.Run(args, os.Stdout, os.Stderr, doctor.DefaultDeps())
		},
	})
}
