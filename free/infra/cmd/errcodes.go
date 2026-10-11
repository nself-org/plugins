// Purpose: the error codes of the infra plugin and the mapping from a failure
// to an output.ErrorDetail (code, message, remediation, exit class).
//
// Inputs: an error returned by internal/infra or internal/server, or one of the
// typed errors of this package (usageError, tokenError).
//
// Outputs: output.ErrorDetail for output.WriteError. The exit status is derived
// from the class (user 1, infra 2, auth 3, destructive_blocked 4).
//
// Constraints: a plugin cannot register codes in the core registry, so it
// reuses two core codes and owns E740 to E759. Messages are fixed text: they
// never echo a token, an env value or a raw provider or terraform error; Cause
// is limited to the terraform exit status and the HTTP status of a Hetzner
// reply.
//
//	E401 usage: bad or missing flag, unknown or unshipped provider   user
//	E403 destructive action blocked: a consent gate said no          destructive_blocked
//	E740 terraform or tofu binary not on PATH                        infra
//	E741 terraform command failed                                    infra
//	E742 no Hetzner API token found                                  auth
//	E743 --token-env names the Terraform-only variable (refused)     user
//	E744 Hetzner Cloud operation failed                              infra
//	E745 server not found                                            user
//	E746 disk shrink refused (Hetzner cannot shrink a disk)          user
//	E747 Hetzner rejected the token (HTTP 401 or 403)                auth
//	E749 unexpected failure                                          infra
package main

import (
	"errors"
	"os/exec"
	"regexp"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/output"
	"github.com/nself-org/nself-infra/internal/infra"
	"github.com/nself-org/nself-infra/internal/server"
)

// Error codes.
const (
	CodeUsage          = "E401"
	CodeBlocked        = "E403"
	CodeTerraformGone  = "E740"
	CodeTerraformFail  = "E741"
	CodeNoToken        = "E742"
	CodeTokenEnvBanned = "E743"
	CodeHetznerFailed  = "E744"
	CodeServerNotFound = "E745"
	CodeDiskShrink     = "E746"
	CodeTokenRejected  = "E747"
	CodeUnexpected     = "E749"
)

// usageError is a usage problem found by this package; msg is the human message.
// safe, when set, replaces msg in the envelope (msg may echo a flag value).
type usageError struct{ msg, safe string }

func (e *usageError) Error() string { return e.msg }

// envelopeMsg is the text the envelope carries.
func (e *usageError) envelopeMsg() string {
	if e.safe != "" {
		return e.safe
	}
	return e.msg
}

// tokenError is a failure to resolve the Hetzner token. refused is true when
// --token-env named the Terraform-only variable.
type tokenError struct {
	refused bool
	err     error
}

func (e *tokenError) Error() string { return e.err.Error() }
func (e *tokenError) Unwrap() error { return e.err }

func usageDetail(msg string) output.ErrorDetail {
	return output.ErrorDetail{Code: CodeUsage, Message: msg, Class: output.ClassUser,
		Remediation: "run nself infra <command> --help"}
}

// blockedDetail is the refusal of a destructive or spending action.
func blockedDetail(msg, remediation string) output.ErrorDetail {
	return output.ErrorDetail{Code: CodeBlocked, Class: output.ClassDestructiveBlocked,
		Message: msg, Remediation: remediation}
}

// httpRe pulls the status and path out of a Hetzner client error text.
var httpRe = regexp.MustCompile(`hetzner API [A-Z]+ (\S+): (\d{3})\b`)

// classify maps err to its error detail. Order matters: the typed errors first.
// sub is the subcommand ("apply", "server list"), used in the terraform message.
func classify(err error, sub string) output.ErrorDetail {
	var te *tokenError
	var ue *usageError
	var ee *exec.ExitError
	text := err.Error()
	switch {
	case errors.As(err, &ue):
		return usageDetail(ue.envelopeMsg())
	case errors.As(err, &te) && te.refused:
		return output.ErrorDetail{Code: CodeTokenEnvBanned, Class: output.ClassUser,
			Message:     "--token-env names the Terraform-only token variable, which a server command never reads",
			Remediation: "export the token in your shell and name it with --token-env, or pass --token"}
	case errors.As(err, &te):
		return output.ErrorDetail{Code: CodeNoToken, Class: output.ClassAuth,
			Message:     "no Hetzner API token was found",
			Remediation: "set HETZNER_NSELF_TOKEN (or HCLOUD_TOKEN) in the environment, or pass --token"}
	case errors.Is(err, server.ErrNoBackup):
		return blockedDetail("destroy refused: no verified backup was requested, the server was not deleted",
			"pass --snapshot to take one first, or --force-no-backup to proceed without one")
	case errors.Is(err, server.ErrDiskShrink):
		return output.ErrorDetail{Code: CodeDiskShrink, Class: output.ClassUser,
			Message:     "Hetzner Cloud cannot shrink a server's disk, the resize was refused",
			Remediation: "snapshot the server, provision a smaller one from the snapshot, then destroy the original"}
	case errors.Is(err, infra.ErrTerraformNotFound):
		return output.ErrorDetail{Code: CodeTerraformGone, Class: output.ClassInfra,
			Message: "neither tofu nor terraform was found on PATH", Remediation: "install OpenTofu (https://opentofu.org) or Terraform"}
	case errors.As(err, &ee):
		return output.ErrorDetail{Code: CodeTerraformFail, Class: output.ClassInfra, Cause: ee.Error(),
			Message: "the terraform run for infra " + sub + " failed", Remediation: "terraform's own output is on stderr"}
	}
	if m := httpRe.FindStringSubmatch(text); m != nil {
		if m[2] == "401" || m[2] == "403" {
			return output.ErrorDetail{Code: CodeTokenRejected, Class: output.ClassAuth, Cause: "HTTP " + m[2],
				Message:     "Hetzner Cloud rejected the API token",
				Remediation: "check that the token is valid, not revoked, and belongs to the right Hetzner project (HETZNER_NSELF_TOKEN, HCLOUD_TOKEN or --token); a read-only token cannot change servers"}
		}
		if m[2] == "404" && strings.HasPrefix(m[1], "/servers/") {
			return output.ErrorDetail{Code: CodeServerNotFound, Class: output.ClassUser, Cause: "HTTP 404",
				Message: "the server was not found", Remediation: "check the id with nself infra server list"}
		}
		return hetznerDetail("HTTP "+m[2], text)
	}
	// internal/server names the missing flag in its own validation errors; the
	// commands wrap them ("provision web: provision: --name is required").
	for e := err; e != nil; e = errors.Unwrap(e) {
		if msg, ok := usageText(e.Error()); ok {
			return usageDetail(msg)
		}
	}
	if strings.Contains(text, "hetzner API ") && strings.Contains(text, "network error") {
		return hetznerDetail("network error", text)
	}
	if strings.HasPrefix(text, "infra: terraform ") {
		return output.ErrorDetail{Code: CodeTerraformFail, Class: output.ClassInfra,
			Message: "the terraform run for infra " + sub + " failed", Remediation: "terraform's own output is on stderr"}
	}
	d := output.ErrorDetail{Code: CodeUnexpected, Class: output.ClassInfra, Message: "the infra command failed unexpectedly"}
	if strings.Contains(text, "NOT deleted") {
		d.Message += ", the server was not deleted"
	}
	return d
}

// hetznerDetail is E744. The message never carries the provider's text.
func hetznerDetail(cause, text string) output.ErrorDetail {
	d := output.ErrorDetail{Code: CodeHetznerFailed, Class: output.ClassInfra, Cause: cause,
		Message:     "the Hetzner Cloud operation failed",
		Remediation: "check the server with nself infra server list before retrying"}
	if strings.Contains(text, "NOT deleted") {
		d.Message = "the Hetzner Cloud operation failed, the server was not deleted"
	}
	return d
}

// knownUsage is the set of internal/server validation messages, which name flags
// only. Matching them exactly keeps a provider error that merely contains
// " is required" from being echoed.
var knownUsage = map[string]bool{
	"provision: --name is required":                      true,
	"provision: --type is required (e.g. cx22)":          true,
	"provision: --location is required (e.g. fsn1)":      true,
	"provision: --image is required (e.g. ubuntu-24.04)": true,
	"resize: server ID is required":                      true,
	"resize: --type is required (target server type)":    true,
	"destroy: server ID is required":                     true,
}

// usageText reports whether text is one of internal/server's own validation
// messages, and the envelope text for it. An unknown target type is reported
// without the value.
func usageText(text string) (string, bool) {
	if knownUsage[text] {
		return text, true
	}
	if strings.HasPrefix(text, "resize: unknown target server type") {
		return "resize: unknown target server type", true
	}
	return "", false
}

var (
	invalidArgRe = regexp.MustCompile(`^invalid argument .* for "([^"]+)" flag`)
	flagCutRe    = regexp.MustCompile(`^((?:unknown flag: |flag needs an argument: )--?[^=\s]*)`)
)

// safeUsage reduces a cobra or pflag error to text that cannot carry a flag
// value: the flag name only. A mistyped --tokn=SECRET, -tSECRET or --id SECRET
// must not reach stdout.
func safeUsage(msg string) string {
	short := strings.HasPrefix(msg, "unknown shorthand flag") || strings.HasPrefix(msg, "flag needs an argument: '")
	if i := strings.Index(msg, " in -"); short && i > 0 {
		return msg[:i]
	}
	if m := invalidArgRe.FindStringSubmatch(msg); m != nil {
		return "invalid value for flag " + m[1]
	}
	if m := flagCutRe.FindStringSubmatch(msg); m != nil {
		return m[1]
	}
	return "invalid usage"
}
