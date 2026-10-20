package run

// Purpose: a hosted dispatch the provider refused for a reason a re-run cannot fix is not an
//          infrastructure loss (P7-HOST-11, Epic D13). A credential or permission refusal (E700,
//          E701) and a misconfigured request (E702) end the attempt as a coded failure that needs
//          the operator; a transient or outage error keeps the bounded infra retry (CI-40).
// Inputs:  the error the carrier ended with.
// Outputs: whether it is such a refusal, and the finished JobOutcome for one.
// Constraints: only providers.Error values of class auth or config with these codes count; every
//          other carrier error (E704, E708 lease races, ssh) takes the existing lost path.

import (
	"errors"

	"github.com/nself-org/plugins/free/ci/internal/providers"
)

// providerRefusal returns the provider error when cause is a refusal a retry cannot fix.
func providerRefusal(cause error) (*providers.Error, bool) {
	var pe *providers.Error
	if !errors.As(cause, &pe) {
		return nil, false
	}
	switch pe.Code {
	case "E700", "E701", "E702":
		return pe, pe.Class == providers.Auth || pe.Class == providers.Config
	}
	return nil, false
}

// refusedOutcome settles a lost attempt whose dispatch was refused: failure class code, not
// retryable, left for the operator.
func (rs *runState) refusedOutcome(id string, o JobOutcome, pe *providers.Error) JobOutcome {
	o.FailureClass, o.Code, o.retryable = "code", pe.Code, false
	o.Diagnosis, o.Remediation = "provider.refused", "The provider refused the dispatch ("+pe.Code+"); fix the credential, permission or provider configuration, then run it again."
	if _, err := rs.settle(id, "lost", "needs-operator", "code"); err != nil {
		return failed(o, err)
	}
	return o
}
