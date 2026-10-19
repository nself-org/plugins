package toolhook

import (
	"context"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

func TestHookReportsE614AsInfra(t *testing.T) {
	h := hook{}
	if !h.WantsCheck("python-test") || !h.WantsCheck("python-types") || h.WantsCheck("go-test") || h.Wants(model.Job{Kind: "test"}) {
		t.Fatal("hook must select by check only")
	}
	f := cirun.OutcomeFacts{Info: cirun.AttemptInfo{Check: "python-test"}, ExitCode: 2, Output: "error: Failed to spawn: `pytest`"}
	ch, ok := h.Outcome(context.Background(), f)
	if !ok || !ch.Fail || ch.FailureClass != "infra" || !strings.Contains(ch.Excerpt, "E614") || !strings.Contains(ch.Excerpt, "pytest") {
		t.Fatalf("change %+v ok=%v", ch, ok)
	}
	f.Output = "1 failed, 2 passed"
	if _, ok := h.Outcome(context.Background(), f); ok {
		t.Fatal("a real test failure must stay a code failure")
	}
}

func TestHookIsRegistered(t *testing.T) {
	for _, h := range cirun.Registered.Outcomes {
		if _, ok := h.(hook); ok {
			return
		}
	}
	t.Fatal("init must register the hook")
}
