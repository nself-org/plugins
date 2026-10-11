// Purpose: the error codes of the k8s plugin and the mapping from a failure to
// an output.ErrorDetail (code, message, remediation, exit class).
//
// Inputs: an error returned by internal/k8s or internal/values.
//
// Outputs: output.ErrorDetail for output.WriteError. The exit status is derived
// from the class (user 1, infra 2, destructive_blocked 4).
//
// Constraints: a plugin cannot register codes in the core registry, so it
// reuses two core codes and owns E720 to E739. Messages are fixed text: they
// never echo an error string that could carry a secret, a licence key, an env
// value or helm's release JSON; Cause is limited to the exec exit status.
//
//	E401 usage: bad or missing flag, extra argument         user
//	E403 destructive action blocked: uninstall unconfirmed  destructive_blocked
//	E720 helm not on PATH                                   infra
//	E721 generated values missing                           user
//	E722 helm command failed                                infra
//	E723 values parity differs from the compose model       user
//	E724 compose model could not be resolved                infra
//	E725 helm release not found                             user
//	E726 helm status output unreadable                      infra
//	E727 generated values unusable (mode, unreadable, invalid) user
//	E729 unexpected failure                                 infra
package main

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/output"
	"github.com/nself-org/nself-k8s/internal/k8s"
	"github.com/nself-org/nself-k8s/internal/values"
)

// Error codes.
const (
	CodeUsage            = "E401"
	CodeBlocked          = "E403"
	CodeHelmMissing      = "E720"
	CodeValuesMissing    = "E721"
	CodeHelmFailed       = "E722"
	CodeParity           = "E723"
	CodeComposeFailed    = "E724"
	CodeReleaseNotFound  = "E725"
	CodeStatusUnreadable = "E726"
	CodeValuesUnusable   = "E727"
	CodeUnexpected       = "E729"
)

func usageDetail(msg string) output.ErrorDetail {
	return output.ErrorDetail{Code: CodeUsage, Message: msg, Class: output.ClassUser,
		Remediation: "run nself k8s <command> --help"}
}

// blockedDetail is the refusal of an unconfirmed uninstall.
func blockedDetail(release string) output.ErrorDetail {
	return output.ErrorDetail{Code: CodeBlocked, Class: output.ClassDestructiveBlocked,
		Message:     fmt.Sprintf("uninstall of release %q needs confirmation: nothing was removed", release),
		Remediation: "run it again with --yes to remove the release"}
}

// classify maps err to its error detail. Order matters: the typed errors first.
func classify(err error) output.ErrorDetail {
	var he *k8s.HelmError
	var ve *values.Error
	var ue *k8s.ValuesError
	switch {
	case errors.Is(err, k8s.ErrHelmNotFound):
		return output.ErrorDetail{Code: CodeHelmMissing, Class: output.ClassInfra,
			Message: "the helm binary was not found on PATH", Remediation: "install helm from https://helm.sh"}
	case errors.Is(err, k8s.ErrValuesMissing):
		return output.ErrorDetail{Code: CodeValuesMissing, Class: output.ClassUser,
			Message: "the generated chart values were not found", Remediation: "run nself k8s values"}
	case errors.As(err, &ue):
		return output.ErrorDetail{Code: CodeValuesUnusable, Class: output.ClassUser,
			Message: "the generated chart values cannot be used", Remediation: "check the mode of secrets.yaml (chmod 600) or run nself k8s values again"}
	case errors.Is(err, k8s.ErrStatusUnparsable):
		return output.ErrorDetail{Code: CodeStatusUnreadable, Class: output.ClassInfra,
			Message: "helm status printed something that is not a release document"}
	case errors.As(err, &he):
		return helmDetail(he)
	case errors.As(err, &ve):
		return valuesDetail(ve, err)
	}
	return output.ErrorDetail{Code: CodeUnexpected, Class: output.ClassInfra, Message: "the k8s command failed unexpectedly"}
}

func helmDetail(he *k8s.HelmError) output.ErrorDetail {
	if he.NotFound {
		return output.ErrorDetail{Code: CodeReleaseNotFound, Class: output.ClassUser,
			Message: "the helm release was not found", Remediation: "run nself k8s install, or pass --release and --cluster for the right release"}
	}
	d := output.ErrorDetail{Code: CodeHelmFailed, Class: output.ClassInfra,
		Message: "helm " + he.Verb + " failed", Remediation: "helm's own output is on stderr"}
	var ee *exec.ExitError
	if errors.As(he.Err, &ee) {
		d.Cause = ee.Error()
	}
	return d
}

func valuesDetail(ve *values.Error, err error) output.ErrorDetail {
	switch ve.Kind {
	case values.KindCompose:
		return output.ErrorDetail{Code: CodeComposeFailed, Class: output.ClassInfra,
			Message: "the compose model could not be resolved", Remediation: "run nself build, then check that docker compose config works in the project"}
	case values.KindMissing:
		return output.ErrorDetail{Code: CodeValuesMissing, Class: output.ClassUser,
			Message: "the generated chart values were not found", Remediation: "run nself k8s values"}
	case values.KindInvalid:
		return output.ErrorDetail{Code: CodeValuesUnusable, Class: output.ClassUser,
			Message: "the existing values.yaml cannot be read as chart values", Remediation: "run nself k8s values again"}
	case values.KindParity:
		return output.ErrorDetail{Code: CodeParity, Class: output.ClassUser,
			Message: strings.TrimPrefix(err.Error(), "parity check failed: "), Remediation: "run nself k8s values to regenerate the values"}
	}
	return output.ErrorDetail{Code: CodeUnexpected, Class: output.ClassInfra, Message: "the values run failed"}
}
