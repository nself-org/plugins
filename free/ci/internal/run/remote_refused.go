package run

// Purpose: a hosted dispatch the provider refused for a reason a re-run cannot fix is not an
//          infrastructure loss (P7-HOST-11, Epic D13). A credential or permission refusal (E700,
//          E701) and a misconfigured request (E702) end the attempt as a coded failure that needs
//          the operator; a transient or outage error keeps the bounded infra retry (CI-40).
// Inputs:  the provider error code the carrier ended with (providerRefusal, in trust_hook.go, decides).
// Outputs: the finished JobOutcome of a refused dispatch.
// Constraints: --confirm-flaky never reruns a refused dispatch (JobOutcome.refused); only providers.Error values of class auth or config with these codes count; every
//          other carrier error (E704, E708 lease races, ssh) takes the existing lost path.

// refusedOutcome settles a lost attempt whose dispatch was refused: failure class code, not
// retryable, left for the operator.
func (rs *runState) refusedOutcome(id string, o JobOutcome, code string) JobOutcome {
	o.FailureClass, o.Code, o.retryable, o.refused = "code", code, false, true
	o.Diagnosis, o.Remediation = "provider.refused", "The provider refused the dispatch ("+code+"); fix the credential, permission or provider configuration, then run it again."
	if _, err := rs.settle(id, "lost", "needs-operator", "code"); err != nil {
		return failed(o, err)
	}
	return o
}
