package main

// Purpose: the flags of the one-shot ad-hoc job `nself ci [flags] -- <cmd> [args…]`
// (P7-CI-52, Epic D22) and the image flag of `--container`.
// Inputs: argv, through bareFlags. Outputs: adhoc.SetFlags for the handler
// (internal/adhoc, registered from its init) and NSELF_CI_IMAGE for the image
// hook of `--container` (P7-CI-43 reads the image from that variable).
// Constraints: no new subcommand (the ad-hoc job is the bare form), so the
// first-level command list is unchanged; flags are registered from init() so
// cmd/v15.go stays unedited; `--trust` moves to the handler because
// run.Options.Validate demands a trust classifier for any --trust value and
// the ad-hoc job fails closed on its own (E647); v1.4 mode never reaches here.

import (
	"os"

	"github.com/nself-org/plugins/free/ci/internal/adhoc"
	"github.com/nself-org/plugins/free/ci/internal/toolchain"
)

// adhocFlags holds the flag values of the current invocation; adhocOptions
// reads and clears them (a process runs one invocation; tests parse several).
var adhocFlags adhoc.Flags

func init() {
	bareFlags.StringVar(&adhocFlags.Isolation, "isolation", "", "isolation of a `-- <cmd>` job: container (default), process (host, with a warning), sandboxed-container")
	bareFlags.StringVar(&adhocFlags.Image, "image", "", "container image pinned by digest (ref@sha256:<64 hex>) for --container and `-- <cmd>` jobs")
	RegisterOptionsHook(adhocOptions)
}

func adhocOptions(o *RunOptions) error {
	fl := adhocFlags
	adhocFlags = adhoc.Flags{}
	if fl.Image != "" {
		if err := os.Setenv(toolchain.EnvImage, fl.Image); err != nil {
			return err
		}
	}
	if len(o.Adhoc) == 0 {
		if fl.Isolation != "" {
			return adhoc.UsageError("--isolation applies to `nself ci -- <cmd>` only")
		}
		return nil
	}
	fl.Trust, o.Trust = o.Trust, ""
	adhoc.SetFlags(fl)
	return nil
}
