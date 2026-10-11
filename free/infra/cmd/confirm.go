// Purpose: the consent gate shared by `infra apply` and `infra destroy`
// (CANON-12, DEPL D9): cloud spend must be accepted on the command line and
// confirmed from a terminal before anything reaches terraform.
//
// Inputs: the --i-accept-cloud-spend value and stdin.
//
// Outputs: nil when both checks pass; otherwise the refusal (exit 4, an E403
// envelope under --json) and the command stops before terraform is touched.
//
// Constraints: the order is fixed, flag first, terminal second, and both run
// before any argument is validated or any provider code is reached. The
// terminal check sits behind a variable so tests can drive both branches
// without a pty; the production default is the isatty ioctl (stdlib only).
package main

import (
	"fmt"
	"os"
)

// stdinIsTTY reports whether stdin is an interactive terminal. Tests replace it.
var stdinIsTTY = func() bool { return isTerminal(os.Stdin) }

// isTerminal reports whether f is a terminal, by the isatty ioctl (tty_*.go).
// A character-device test is not enough: /dev/null (what exec gives a child by
// default), /dev/zero and /dev/urandom are character devices and not terminals.
func isTerminal(f *os.File) bool {
	return isatty(f.Fd())
}

// spendGate runs the two spend checks for the subcommand verb ("apply" or "destroy").
func (r *invocation) spendGate(verb string, accepted bool) error {
	if !accepted {
		return r.refuse(
			fmt.Sprintf("nself infra %s will incur cloud spend. Pass --i-accept-cloud-spend to proceed.", verb),
			fmt.Sprintf("nself infra %s needs --i-accept-cloud-spend: nothing was changed", verb),
			"pass --i-accept-cloud-spend")
	}
	if !stdinIsTTY() {
		return r.refuse(
			fmt.Sprintf("nself infra %s requires a TTY to confirm spend.", verb),
			fmt.Sprintf("nself infra %s needs an interactive terminal to confirm cloud spend: nothing was changed", verb),
			"run it from a terminal")
	}
	return nil
}

// shippedElsewhere lists the providers that are recognised but not shipped (DEF-005).
func shippedElsewhere(p string) bool {
	return p == "aws" || p == "gcp" || p == "azure" || p == "do" || p == "linode"
}

// notShipped reports a recognised-but-unshipped provider: human mode prints the
// line it always printed and exits 1; JSON mode writes an E401 envelope.
func (r *invocation) notShipped(provider string) error {
	if r.json {
		return r.usage("provider " + provider + " is not shipped (DEF-005); supported: hetzner")
	}
	fmt.Fprintf(os.Stderr, "not shipped (DEF-005)\n")
	return r.silentExit(1)
}
