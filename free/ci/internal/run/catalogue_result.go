package run

import (
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/gates"
)

// classifyCatalogue applies the same missing-tool verdict as the direct
// catalogue runner. It cannot turn a cancellation or a successful job into a failure.
func classifyCatalogue(check string, res *ciexec.AttemptResult, o *JobOutcome) {
	if res.Attempt.State != "failed" {
		return
	}
	tool, fix := gates.MissingResult(check, res.ExitCode, o.Tail)
	if tool == "" {
		return
	}
	res.Attempt.FailureClass = "infra"
	res.Err = &Error{Code: "E614", Message: tool + " unavailable; " + fix}
	o.Tool, o.Code, o.FailureClass = tool, "E614", "infra"
	o.Remediation, o.retryable = fix, false
	o.Excerpt = res.Err.Error()
}
