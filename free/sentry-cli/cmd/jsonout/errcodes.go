// Purpose: the error codes of the sentry-cli plugin and the mapping from a
// failure to an output.ErrorDetail (code, message, remediation, exit class).
//
// Inputs: an error returned by a sentry command, by internal/sentryapi, or one
// of the typed errors of this package (UsageErr).
//
// Outputs: output.ErrorDetail for output.WriteError. The exit status derives
// from the class (user 1, infra 2, auth 3, destructive_blocked 4).
//
// Constraints: a plugin cannot register codes in the core registry, so it
// reuses E401 and owns E780 to E789 (E740 to E779 belong to infra, functions
// and plugin-dev). Messages are fixed text: they never echo an API key, a URL,
// a token or a raw upstream error; Cause is limited to an HTTP status or a
// fixed step name.
//
//	E401 usage: bad or missing flag or argument                     user
//	E780 ɳSentry API key missing or rejected                        auth
//	E781 plan quota exceeded                                        user
//	E782 ɳSentry API request failed                                 infra
//	E783 status page fetch failed                                   infra
//	E784 test notification not delivered                            infra
//	E785 local credentials file could not be written or removed     user
//	E786 sentry-server provision step failed                        infra
//	E789 unexpected failure                                         infra
package jsonout

import (
	"errors"
	"regexp"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/output"
	"github.com/nself-org/nself-sentry-cli/internal/sentryapi"
)

// Error codes.
const (
	CodeUsage      = "E401"
	CodeAuth       = "E780"
	CodeQuota      = "E781"
	CodeAPI        = "E782"
	CodeStatusPage = "E783"
	CodeNotSent    = "E784"
	CodeCredsFile  = "E785"
	CodeProvision  = "E786"
	CodeUnexpected = "E789"
)

// UsageErr is a usage problem found by a command. Msg is the human text (it may
// echo a flag value); Safe replaces it in the envelope when set.
type UsageErr struct{ Msg, Safe string }

func (e *UsageErr) Error() string { return e.Msg }

var (
	httpRe      = regexp.MustCompile(`HTTP ([0-9]{3})`)
	provStepRe  = regexp.MustCompile(`^sentry-server provision: (terraform|SSH wait|push secrets|ops deploy)`)
	invalidArg  = regexp.MustCompile(`^invalid argument .* for "([^"]+)" flag`)
	flagCutRe   = regexp.MustCompile(`^((?:unknown flag: |flag needs an argument: )--?[^=\s]*)`)
	apiHTTPText = "sentry api HTTP "
)

// Classify maps err to the envelope error object for command sub.
func Classify(err error, sub string) output.ErrorDetail {
	text := err.Error()
	var ue *UsageErr
	switch {
	case errors.As(err, &ue):
		m := ue.Safe
		if m == "" {
			m = ue.Msg
		}
		return output.ErrorDetail{Code: CodeUsage, Class: output.ClassUser, Message: m}
	case errors.Is(err, sentryapi.ErrUnauthorized), strings.HasPrefix(text, "API key rejected by "):
		return output.ErrorDetail{Code: CodeAuth, Class: output.ClassAuth,
			Message: "the ɳSentry API key is missing or was rejected", Remediation: "run nself sentry login with a valid nsk_ key"}
	case errors.Is(err, sentryapi.ErrQuotaExceeded):
		return output.ErrorDetail{Code: CodeQuota, Class: output.ClassUser,
			Message: "the quota of your ɳSentry plan is exceeded", Remediation: "upgrade at https://sentry.nself.org/billing or remove unused items"}
	case strings.HasPrefix(text, "invalid API key"):
		return output.ErrorDetail{Code: CodeUsage, Class: output.ClassUser, Message: "the API key format is invalid: it must start with nsk_ and be longer"}
	case strings.HasPrefix(text, "test notification NOT delivered"):
		return output.ErrorDetail{Code: CodeNotSent, Class: output.ClassInfra, Message: "the test notification was not delivered",
			Remediation: "check the channel target in the ɳSentry dashboard"}
	case strings.HasPrefix(text, "saving credentials"), strings.HasPrefix(text, "logout:"):
		return output.ErrorDetail{Code: CodeCredsFile, Class: output.ClassUser, Message: "the local ɳSentry credentials file could not be updated",
			Remediation: "check the permissions of ~/.nself/sentry.json"}
	case strings.Contains(text, apiHTTPText):
		return output.ErrorDetail{Code: CodeAPI, Class: output.ClassInfra, Cause: httpCause(text), Message: "the ɳSentry API request failed"}
	case strings.Contains(text, "sentry api unreachable"), strings.HasPrefix(text, "validating key against"):
		return output.ErrorDetail{Code: CodeAPI, Class: output.ClassInfra, Cause: "network error", Message: "the ɳSentry API could not be reached",
			Remediation: "check --api-url and the network"}
	case strings.HasPrefix(text, "parse response"), strings.HasPrefix(text, "read response"), strings.HasPrefix(text, "build request"):
		return output.ErrorDetail{Code: CodeAPI, Class: output.ClassInfra, Message: "the ɳSentry API request failed"}
	case isStatusFetch(text):
		return output.ErrorDetail{Code: CodeStatusPage, Class: output.ClassInfra, Cause: httpCause(text), Message: "the status page could not be fetched",
			Remediation: "check --url and --token"}
	case strings.HasPrefix(text, "invalid --url"):
		return output.ErrorDetail{Code: CodeUsage, Class: output.ClassUser, Message: "invalid --url"}
	}
	if m := provStepRe.FindStringSubmatch(text); m != nil {
		return output.ErrorDetail{Code: CodeProvision, Class: output.ClassInfra, Cause: m[1], Message: "a sentry-server provision step failed",
			Remediation: "re-run with --dry-run to review the steps; step output is on stderr"}
	}
	if strings.HasPrefix(text, "sentry-server provision:") {
		return output.ErrorDetail{Code: CodeProvision, Class: output.ClassInfra, Message: "sentry-server provision could not continue",
			Remediation: "re-run with --host to deploy to an existing server"}
	}
	return output.ErrorDetail{Code: CodeUnexpected, Class: output.ClassInfra, Message: "the " + sub + " command failed unexpectedly"}
}

// isStatusFetch reports whether text is a status page fetch failure.
func isStatusFetch(text string) bool {
	for _, p := range []string{"fetching status page", "status page returned HTTP", "reading response body", "parsing status JSON", "building request"} {
		if strings.HasPrefix(text, p) {
			return true
		}
	}
	return false
}

// httpCause returns "HTTP nnn" when text names an HTTP status, else "".
func httpCause(text string) string {
	if m := httpRe.FindStringSubmatch(text); m != nil {
		return "HTTP " + m[1]
	}
	return ""
}

// SafeUsage reduces a cobra or pflag error to text that cannot carry a flag
// value: a mistyped --tokn=SECRET must not reach stdout.
func SafeUsage(msg string) string {
	short := strings.HasPrefix(msg, "unknown shorthand flag") || strings.HasPrefix(msg, "flag needs an argument: '")
	if i := strings.Index(msg, " in -"); short && i > 0 {
		return msg[:i]
	}
	if m := invalidArg.FindStringSubmatch(msg); m != nil {
		return "invalid value for flag " + m[1]
	}
	if m := flagCutRe.FindStringSubmatch(msg); m != nil {
		return m[1]
	}
	if strings.HasPrefix(msg, "accepts ") || strings.HasPrefix(msg, "required flag") {
		if i := strings.Index(msg, ", received"); i > 0 {
			return msg[:i]
		}
		return msg
	}
	return "invalid usage"
}
