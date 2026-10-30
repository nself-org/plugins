package run

import (
	"context"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestRaceableNeverAtMostOnce(t *testing.T) {
	no, yes := false, true
	cases := map[string]struct {
		job  *model.Job
		want bool
	}{
		"nil":            {nil, false},
		"plain":          {&model.Job{}, true},
		"idempotent":     {&model.Job{Idempotent: &yes}, true},
		"release":        {&model.Job{Kind: "release"}, false},
		"not idempotent": {&model.Job{Idempotent: &no}, false},
	}
	for name, c := range cases {
		if got := Raceable(c.job); got != c.want {
			t.Errorf("%s: Raceable = %v, want %v", name, got, c.want)
		}
	}
}

func TestRaceAttemptsRefuseRelease(t *testing.T) {
	ra := RaceAttempts{Spec: AttemptSpec{Job: &model.Job{Kind: "release"}}}
	if _, err := ra.Create(context.Background(), 2, "n1"); err == nil {
		t.Fatal("a sibling of a release job was created")
	}
}

func TestRaceAttemptsNeedSession(t *testing.T) {
	ra := RaceAttempts{}
	if err := ra.Cancel(context.Background(), "a", 1, "lost"); err == nil {
		t.Fatal("cancel without a session succeeded")
	}
	if _, _, err := ra.State(context.Background(), "a"); err == nil {
		t.Fatal("state without a session succeeded")
	}
}
