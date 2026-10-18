package main

// Purpose: the error codes of the plugin-dev plugin and the mapping from a
// failure to an output.ErrorDetail (code, message, remediation, exit class).
// Inputs:  an error returned by a command body; the ones this plugin raises on
// purpose are *codedError values made by coded().
// Outputs: output.ErrorDetail for output.WriteError. The exit status comes from
// the class (user 1, infra 2).
// Constraints: a plugin cannot register codes in the core registry, so it reuses
// E401 and owns E770 to E779 (E740 to E759 are infra, E760 to E765 and E769 functions).
// Messages are fixed text: they never echo a path, a flag value, an env value or
// a raw child-process error. Cause is limited to "exit <n>" of a child process.
//
//	E401 usage: bad or missing flag, argument or value                  user
//	E770 plugin not found                                               user
//	E771 path is not a plugin directory (no plugin.yaml)                user
//	E772 scaffold destination is not empty                              user
//	E773 plugin tests failed                                            user
//	E774 plugin-links.json could not be read or written                 infra
//	E775 scaffold failed (render or write)                              infra
//	E776 a build, debugger, watcher, install step or test tool failed   infra
//	E779 unexpected failure                                             infra

import (
	"errors"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/output"
)

// Error codes.
const (
	CodeUsage          = "E401"
	CodeNotFound       = "E770"
	CodeNotPluginDir   = "E771"
	CodeDestNotEmpty   = "E772"
	CodeTestsFailed    = "E773"
	CodeLinksIO        = "E774"
	CodeScaffoldFailed = "E775"
	CodeStepFailed     = "E776"
	CodeUnexpected     = "E779"
)

// codedError carries the envelope detail next to the original error, so the
// human report ("Error: <err>") stays the text core printed.
type codedError struct {
	err    error
	detail output.ErrorDetail
}

func (e *codedError) Error() string { return e.err.Error() }
func (e *codedError) Unwrap() error { return e.err }

// coded attaches an envelope detail to err. A nil err stays nil.
func coded(err error, code, class, msg, remediation string) error {
	if err == nil {
		return nil
	}
	return &codedError{err: err, detail: output.ErrorDetail{
		Code: code, Class: class, Message: msg, Remediation: remediation}}
}

// usageErr is a usage error whose human text is err and whose envelope text is
// the fixed msg (err may echo a flag value).
func usageErr(err error, msg string) error {
	return coded(err, CodeUsage, output.ClassUser, msg, "run nself plugin-dev <command> --help")
}

// stepErr wraps the failure of an external step (go build, dlv, the watcher,
// nself plugin install). Cause is the child's exit status when it has one.
func stepErr(err error, msg, remediation string) error {
	ce := coded(err, CodeStepFailed, output.ClassInfra, msg, remediation)
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		ce.(*codedError).detail.Cause = "exit " + strconv.Itoa(ee.ExitCode())
	}
	return ce
}

// classify maps err to its error detail. Uncoded errors are E779.
func classify(err error) output.ErrorDetail {
	var ce *codedError
	if errors.As(err, &ce) {
		return ce.detail
	}
	return output.ErrorDetail{Code: CodeUnexpected, Class: output.ClassInfra,
		Message: "the plugin-dev command failed unexpectedly"}
}

var (
	invalidArgRe = regexp.MustCompile(`^invalid argument .* for "([^"]+)" flag`)
	flagCutRe    = regexp.MustCompile(`^((?:unknown flag: |flag needs an argument: )--?[^=\s]*)`)
)

// safeUsage reduces a parser error to text that cannot carry a flag value: the
// flag name only. A mistyped --tokn=SECRET must not reach stdout.
func safeUsage(msg string) string {
	if strings.HasPrefix(msg, "unknown shorthand flag") {
		return "unknown shorthand flag"
	}
	if m := invalidArgRe.FindStringSubmatch(msg); m != nil {
		return "invalid value for flag " + m[1]
	}
	if m := flagCutRe.FindStringSubmatch(msg); m != nil {
		return m[1]
	}
	if strings.HasPrefix(msg, "accepts ") {
		return msg // "accepts 1 arg(s), received 2": counts only
	}
	return "invalid usage"
}
