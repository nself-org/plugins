package evidence

import (
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// TestVerifyRunnerFieldsAreBound: the runner a document names is part of what is recorded and
// verified; a document edited to name another node, platform or isolation no longer matches
// the record of its run and is invalid, while the untouched one verifies.
func TestVerifyRunnerFieldsAreBound(t *testing.T) {
	cur := sampleInputs()
	remote := func() model.Evidence {
		ev := sampleEvidence()
		ev.Runner = model.RunnerRef{ID: "lab-arm", Provider: "ssh", Platform: "linux/arm64", Isolation: "container"}
		ev.Attempts = []model.AttemptEvidence{{JobID: "unit", Attempt: 1, Runner: model.AttemptRunner{Kind: "ssh", Tier: "lan", Node: "lab-arm", Provider: "ssh", Platform: "linux/arm64", Isolation: "container"}}}
		return ev
	}
	for name, ev := range map[string]model.Evidence{"local": sampleEvidence(), "remote": remote()} {
		stored := ev
		same := func(model.Evidence) *model.Evidence { return &stored }
		if v, err := VerifyDocument(docOf(t, ev), sampleRev, &cur, same); err != nil || v.Exit != ExitOK {
			t.Fatalf("%s: the recorded document must verify: %+v %v", name, v, err)
		}
	}
	stored := remote()
	same := func(model.Evidence) *model.Evidence { return &stored }
	for name, edit := range map[string]func(*model.Evidence){
		"run node":          func(e *model.Evidence) { e.Runner.ID = "local" },
		"run platform":      func(e *model.Evidence) { e.Runner.Platform = "linux/amd64" },
		"run isolation":     func(e *model.Evidence) { e.Runner.Isolation = "process" },
		"attempt node":      func(e *model.Evidence) { e.Attempts[0].Runner.Node = "lab-x86" },
		"attempt isolation": func(e *model.Evidence) { e.Attempts[0].Runner.Isolation = "vm" },
		"attempt dropped":   func(e *model.Evidence) { e.Attempts = nil },
	} {
		tampered := remote()
		edit(&tampered)
		doc := docOf(t, tampered)
		if v, err := VerifyDocument(doc, sampleRev, &cur, same); err != nil || v.Exit != ExitInvalid {
			t.Errorf("%s: a tampered runner must be invalid: %+v %v", name, v, err)
		}
	}
}
