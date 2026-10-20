package evidence

import (
	"github.com/nself-org/plugins/free/ci/internal/model"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"testing"
)

func TestAttemptInfrastructureResult(t *testing.T) {
	for _, class := range []model.FailureClass{"infra", "timeout", "lost"} {
		t.Run(string(class), func(t *testing.T) {
			ev := AttemptDocument(model.Evidence{Checks: []model.Check{{JobID: "job", Required: true, Substantive: true, Result: "error", FailureClass: class}}}, cirun.JobOutcome{ID: "job"})
			if ev.Result != "error" || ev.Counts.Error != 1 || model.Projection(ev).Result != "error" {
				t.Fatalf("incorrect attempt: %+v", ev)
			}
		})
	}
}
