package exec

import (
	"context"
	"errors"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

type refuseIsolator struct{ err error }

func (r refuseIsolator) Prepare(context.Context, *JobSpec) error { return r.err }

// Refused marks only an Isolator refusal, which happens before the workspace exists and anything ran.
func TestRunMarksIsolatorRefusals(t *testing.T) {
	cases := []struct {
		name        string
		iso         Isolator
		command     []string
		wantRefused bool
	}{
		{"isolator refuses", refuseIsolator{errors.New("no runtime")}, []string{"true"}, true},
		{"isolator accepts, job passes", refuseIsolator{}, []string{"true"}, false},
		{"invalid command is not a refusal", refuseIsolator{}, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := (&Executor{Isolation: tc.iso}).Run(context.Background(), JobSpec{AttemptID: "refused", CoordinatorID: "coord", Home: t.TempDir(), Job: model.Job{Isolation: "process"}, Command: tc.command})
			if res.Refused != tc.wantRefused {
				t.Fatalf("Refused = %v, want %v (result %+v)", res.Refused, tc.wantRefused, res)
			}
		})
	}
}
