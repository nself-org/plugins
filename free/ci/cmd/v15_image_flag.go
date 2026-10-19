package main

// Purpose: the `--image <ref@digest>` flag of bare `nself ci` and `nself ci -- <cmd>`
// (D-0993): names the toolchain image `--container` runs in.
// Inputs: argv, through bareFlags. Outputs: NSELF_CI_IMAGE in this process's
// environment, the one input internal/toolchain.Resolver reads.
// Constraints: registered from init() and applied by an options hook, so
// cmd/v15.go stays unedited; the flag wins over an image the environment
// already names; the value is checked by the resolver (E645: not pinned by
// digest), not here; the variable is the operator's own, set before any job
// starts, and job env is built separately.

import (
	"errors"
	"os"

	"github.com/nself-org/plugins/free/ci/internal/toolchain"
)

// imageFlag is the --image value of the invocation being parsed; the hook
// consumes it, so a value never leaks into the next parse.
var imageFlag string

func init() {
	bareFlags.Func("image", "toolchain image for --container, pinned by digest: ref@sha256:<64 hex> (sets "+toolchain.EnvImage+")", func(v string) error {
		if v == "" {
			return errors.New("--image needs a reference pinned by digest (ref@sha256:<64 hex>)")
		}
		imageFlag = v
		return nil
	})
	RegisterOptionsHook(applyImageFlag)
}

// applyImageFlag hands the parsed --image value to the toolchain resolver.
func applyImageFlag(*RunOptions) error {
	v := imageFlag
	imageFlag = ""
	if v == "" {
		return nil
	}
	return os.Setenv(toolchain.EnvImage, v)
}
