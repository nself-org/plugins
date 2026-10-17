package main

// Purpose: the error codes of the functions plugin and the mapping from a
// failure to an output.ErrorDetail (code, message, remediation, exit class).
//
// Inputs: an error returned by a subcommand: a *fnError built where the failure
// happened, a *usageError, or anything else (reported as E769).
//
// Outputs: output.ErrorDetail for output.WriteError. The exit status comes from
// the class (user 1, infra 2, auth 3, destructive_blocked 4).
//
// Constraints: a plugin cannot register codes in the core registry, so it reuses
// two core codes and owns E760 to E769. Envelope text is fixed: it never echoes
// a token, an env value, a flag value, a path or a raw upstream error. Cause is
// limited to the HTTP status of a function reply.
//
//	E401 usage: bad name, method or flag combination             user
//	E403 destructive action blocked: delete without --confirm    destructive_blocked
//	E760 function not found                                      user
//	E761 local file or directory operation failed                infra
//	E762 the functions service did not answer                    infra
//	E763 the function replied with HTTP 400 or above             user, auth or infra
//	E764 docker logs could not be read                           infra
//	E765 deploy source not found                                 user
//	E769 unexpected failure                                      infra
import (
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/output"
)

// Error codes.
const (
	CodeUsage          = "E401"
	CodeBlocked        = "E403"
	CodeNotFound       = "E760"
	CodeFileOp         = "E761"
	CodeServiceDown    = "E762"
	CodeFunctionHTTP   = "E763"
	CodeLogsFailed     = "E764"
	CodeSourceNotFound = "E765"
	CodeUnexpected     = "E769"
)

// fnError carries the envelope detail of a failure next to the original error,
// which human mode prints unchanged and the envelope never contains.
type fnError struct {
	detail output.ErrorDetail
	err    error
}

func (e *fnError) Error() string { return e.err.Error() }
func (e *fnError) Unwrap() error { return e.err }

// newErr wraps err with the envelope detail for it.
func newErr(err error, code, class, msg, remediation string) error {
	return &fnError{err: err, detail: output.ErrorDetail{
		Code: code, Class: class, Message: msg, Remediation: remediation}}
}

// usageError is a usage problem found by this package; msg is the human message
// and safe, when set, replaces it in the envelope (msg may echo a flag value).
type usageError struct{ msg, safe string }

func (e *usageError) Error() string { return e.msg }

func (e *usageError) envelopeMsg() string {
	if e.safe != "" {
		return e.safe
	}
	return e.msg
}

func usageDetail(msg string) output.ErrorDetail {
	return output.ErrorDetail{Code: CodeUsage, Message: msg, Class: output.ClassUser,
		Remediation: "run nself functions <command> --help"}
}

// httpDetail is E763 for a function reply with status >= 400.
func httpDetail(status int) output.ErrorDetail {
	class := output.ClassInfra
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		class = output.ClassAuth
	case status < 500:
		class = output.ClassUser
	}
	return output.ErrorDetail{Code: CodeFunctionHTTP, Class: class, Cause: "HTTP " + strconv.Itoa(status),
		Message:     "the function replied with an HTTP error",
		Remediation: "check the function with nself functions logs <name>"}
}

// classify maps err to its envelope detail.
func classify(err error) output.ErrorDetail {
	var fe *fnError
	var ue *usageError
	switch {
	case errors.As(err, &ue):
		return usageDetail(ue.envelopeMsg())
	case errors.As(err, &fe):
		return fe.detail
	}
	return output.ErrorDetail{Code: CodeUnexpected, Class: output.ClassInfra,
		Message: "the functions command failed unexpectedly"}
}

var (
	invalidArgRe = regexp.MustCompile(`^invalid argument .* for "([^"]+)" flag`)
	flagCutRe    = regexp.MustCompile(`^((?:unknown flag: |flag needs an argument: )--?[^=\s]*)`)
)

// safeUsage reduces a cobra or pflag error to text that cannot carry a flag
// value: the flag name only. A mistyped --auth=SECRET must not reach stdout.
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
	if strings.HasPrefix(msg, "accepts ") || strings.HasPrefix(msg, "requires ") {
		return msg
	}
	return "invalid usage"
}

// fileOpErr is E761 for a failed local file or directory operation.
func fileOpErr(err error) error {
	return newErr(err, CodeFileOp, output.ClassInfra, "a local file or directory operation failed",
		"check the permissions of the project directory")
}
