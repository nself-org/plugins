package run

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func i64(v int64) *int64 { return &v }

// TestEvidenceRunnerTierPool: the attempt records reach the document in a
// stable order, validate against the schema, and are absent when none exist.
func TestEvidenceRunnerTierPool(t *testing.T) {
	start := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	in := evidenceInput{Revision: strings.Repeat("a", 40), Start: start, End: start, Trigger: "local",
		Pipeline:  model.PipelineRef{ID: "default", Name: "default", Kind: "default"},
		Selection: model.Selection{Mode: "full", Reason: "selection.no_base"},
		Attempts: []model.AttemptEvidence{
			{JobID: "test", Attempt: 2, Runner: model.AttemptRunner{Kind: "hosted", Tier: "hosted", Pool: "github", Rank: 2}, Timings: &model.AttemptTimings{QueueMS: i64(5), RunMS: i64(9)}},
			{JobID: "test", Attempt: 1, Runner: model.AttemptRunner{Kind: "ssh", Tier: "owner", Pool: "lab", Rank: 1}},
			{JobID: "build", Attempt: 1, Runner: model.AttemptRunner{Kind: "local", Tier: "owner", Pool: "personal", Rank: 0}},
		},
		Errors: []model.EvidenceError{{Code: "CI-INFRA-OOM", JobID: "test", Attempt: 1}}}
	ev, _, err := buildEvidence(in)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, a := range ev.Attempts {
		got = append(got, a.JobID+"/"+a.Runner.Kind+"/"+a.Runner.Pool)
	}
	if want := "build/local/personal test/ssh/lab test/hosted/github"; strings.Join(got, " ") != want {
		t.Errorf("attempts %v, want %s", got, want)
	}
	schema, err := os.ReadFile(filepath.Join("..", "..", "schemas", "ci-evidence.v1.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	doc, _ := json.Marshal(ev)
	if err := model.ValidateJSON(schema, doc); err != nil {
		t.Errorf("document with attempts: %v", err)
	}
	in.Attempts, in.Errors = nil, nil
	ev, _, _ = buildEvidence(in)
	doc, _ = json.Marshal(ev)
	if strings.Contains(string(doc), `"attempts"`) || strings.Contains(string(doc), `"errors"`) {
		t.Error("without records the additive keys must be absent")
	}
}
