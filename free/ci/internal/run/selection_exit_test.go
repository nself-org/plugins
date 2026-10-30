package run

import (
	"github.com/nself-org/plugins/free/ci/internal/model"
	"testing"
)

func TestSelectedExit(t *testing.T) {
	for _, result := range []model.CheckResult{"pass", "fail", "error", "skip"} {
		checks := []model.Check{{Required: true, Substantive: true, Result: result}, {Required: true, Result: "skip", Reason: "pipeline.excluded"}}
		ev := model.Evidence{Result: runResult(checks), Checks: checks}
		want := exitFor(runResult(checks[:1]))
		if got := selectedExit(ev, true); got != want {
			t.Fatalf("%s: exit %d want %d", result, got, want)
		}
		if got := selectedExit(ev, false); got != exitFor(ev.Result) {
			t.Fatalf("full run: %d", got)
		}
	}
}
