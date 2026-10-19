package run

// D-1060: the coordinator heartbeat must outlive the evidence build. The
// heartbeat used to stop when drive() returned, so a slow evidence builder let
// the row go stale and a polling `ci runs wait` could mark the live run lost.

import (
	"context"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

type slowEvidence struct {
	moved bool
	first int64
}

func (slowEvidence) Begin(context.Context, string, Revision, Capture) any { return nil }

func (s *slowEvidence) Complete(ctx context.Context, _ any, f EvidenceFacts, ev model.Evidence) (model.Evidence, error) {
	beat := func() int64 {
		cs, err := f.Store.Coordinators(ctx)
		if err != nil || len(cs) != 1 {
			return -1
		}
		return cs[0].HeartbeatAt
	}
	s.first = beat()
	for end := time.Now().Add(6 * time.Second); time.Now().Before(end) && !s.moved; time.Sleep(50 * time.Millisecond) {
		s.moved = beat() > s.first
	}
	return ev, nil
}

func TestHeartbeatSurvivesEvidenceBuild(t *testing.T) {
	t.Parallel()
	r := newRig(t, map[string]model.Job{"a": sh(`true`)}, gitFake{})
	r.C.storeTTL = 600 * time.Millisecond
	b := &slowEvidence{}
	r.C.Hooks = Hooks{Evidence: b}
	if res := r.run(t); res.Exit != 0 {
		t.Fatalf("exit %d\n%s", res.Exit, r.Out)
	}
	if b.first <= 0 || !b.moved {
		t.Fatalf("coordinator heartbeat did not advance while the evidence was built (first=%d)", b.first)
	}
}
