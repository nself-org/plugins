package main

// Purpose: the "nodes provision" and "nodes verify" subcommands: build and
// audit self-hosted GitHub Actions runner hosts, ported from core
// `nself runner provision|verify` (P7-CANON-10). The implementation lives in
// internal/nodes/provision; this file registers the two keys in the
// subcommand table (P7-CANON-09) and nothing else, so cmd/main.go is
// untouched and later `nodes <x>` Tickets add their own files.
// Inputs:  the flags of core `nself runner provision` and `nself runner verify`.
// Outputs: provisioning steps or the verify parity matrix on stdout; exit 0,
// or 1 for any failure and for a verify run that finds a failing check, an
// unreachable host or drift, as core.
// Constraints: every ssh call goes through the vendored cli sdk/go/remote;
// this package has no ssh or scp exec site (TestSingleSSHExecSite).

import (
	"github.com/nself-org/plugins/free/ci/internal/nodes/provision"
)

func init() {
	register("nodes provision", func(args []string) int {
		return provision.RunProvision(args, provision.DefaultDeps())
	})
	register("nodes verify", func(args []string) int {
		return provision.RunVerify(args, provision.DefaultDeps())
	})
}
