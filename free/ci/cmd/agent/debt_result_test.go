package main

import (
	"github.com/nself-org/plugins/free/ci/internal/model"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"testing"
	"time"
)

func TestInfrastructureEvidence(t *testing.T) {
	for _, class := range []model.FailureClass{"infra", "timeout", "lost"} {
		t.Run(string(class), func(t *testing.T) {
			ev := baseEvidence(execSpec{}, cirun.JobOutcome{State: cirun.StateFailed, FailureClass: class}, time.Time{}, time.Time{})
			if ev.Checks[0].Result != "error" || ev.Counts.Error != 1 || ev.Result != "error" || model.Projection(ev).Result != "error" {
				t.Fatalf("incorrect infrastructure evidence: %+v", ev)
			}
		})
	}
}
