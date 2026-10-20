package run

import (
	"github.com/nself-org/plugins/free/ci/internal/model"
	"testing"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
)

func TestCatalogueMissingTools(t *testing.T) {
	for _, tc := range []struct {
		check, output, state string
		exit                 int
		missing              bool
	}{
		{"python-test", "No module named pytest", "failed", 1, true},
		{"python-types", "No module named mypy", "failed", 1, true},
		{"xcode-test", "xcode-select: error: tool 'xcodebuild' requires Xcode, but active developer directory is a command line tools instance", "failed", 1, true},
		{"python-test", "assertion failed", "failed", 1, false},
		{"custom", "No module named pytest", "failed", 1, false},
		{"python-test", "No module named pytest", "passed", 0, false},
		{"python-test", "No module named pytest", "cancelled", 1, false},
	} {
		t.Run(tc.check+"/"+tc.state+"/"+tc.output, func(t *testing.T) {
			var res ciexec.AttemptResult
			res.Attempt.State, res.Attempt.FailureClass, res.ExitCode = model.JobState(tc.state), "code", tc.exit
			o := JobOutcome{Tail: tc.output, retryable: true}
			classifyCatalogue(tc.check, &res, &o)
			if (o.Code == "E614") != tc.missing {
				t.Fatalf("code = %q", o.Code)
			}
			if tc.missing && (res.Attempt.FailureClass != "infra" || o.Remediation == "" || o.retryable) {
				t.Fatal("missing tool verdict lost")
			}
		})
	}
}
