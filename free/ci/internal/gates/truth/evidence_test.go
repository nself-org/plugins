package truth

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

const testRevision = "0123456789abcdef0123456789abcdef01234567"

func sweepEntries(t *testing.T) []SweepEntry {
	t.Helper()
	rules := map[string]string{"rules.yaml": fixtureRules()}
	clean := writeTree(t, map[string]string{"rules.yaml": rules["rules.yaml"], "a.md": "fine\n"})
	dirty := writeTree(t, map[string]string{"rules.yaml": rules["rules.yaml"], "a.md": "Old Name\nhost " + plantedHost() + "\n"})
	broken := writeTree(t, map[string]string{"rules.yaml": "version: 9\n"})
	return []SweepEntry{
		{Repo: "clean-repo", Checks: Run(clean, []string{"rules.yaml"}, "")},
		{Repo: "dirty-repo", Checks: Run(dirty, []string{"rules.yaml"}, "")},
		{Repo: ".github", Err: "clone failed: not found"},
		{Repo: "broken-rules", Checks: Run(broken, []string{"rules.yaml"}, "")},
	}
}

// TestSweepEvidenceIsOneValidDocument: one check per repository, a clone
// failure is an error check, and the document passes the verifier and schema.
func TestSweepEvidenceIsOneValidDocument(t *testing.T) {
	ev := SweepEvidence(testRevision, strings.Repeat("a", 64), sweepEntries(t), time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC))
	if len(ev.Checks) != 4 {
		t.Fatalf("want one check per repository, got %d", len(ev.Checks))
	}
	got := byID(ev.Checks)
	for id, want := range map[string]model.CheckResult{
		"truth-sweep.clean-repo": "pass", "truth-sweep.dirty-repo": "fail", "truth-sweep.dot-github": "error", "truth-sweep.broken-rules": "error",
	} {
		if got[id].Result != want {
			t.Errorf("%s = %s, want %s", id, got[id].Result, want)
		}
	}
	if c := got["truth-sweep.dirty-repo"]; c.Findings != 2 || !strings.Contains(c.Excerpt, "truth.retired-term") || strings.Contains(c.Excerpt, plantedHost()) {
		t.Errorf("dirty-repo: %+v", c)
	}
	if ev.Result != "error" {
		t.Errorf("a clone failure must make the document error, got %s", ev.Result)
	}
	doc, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(doc), "\n") {
		t.Error("the document must be one compact line")
	}
	if _, err := attest.Verify(doc); err != nil {
		t.Fatalf("the document is not valid ci.evidence/v1: %v", err)
	}
	schemas, err := model.Schemas()
	if err != nil {
		t.Fatal(err)
	}
	if err := model.ValidateJSON(schemas["ci-evidence.v1.schema.json"], doc); err != nil {
		t.Fatalf("schema: %v", err)
	}
}

func TestSweepEvidencePassesWhenEveryRepoIsClean(t *testing.T) {
	dir := writeTree(t, map[string]string{"rules.yaml": fixtureRules(), "a.md": "fine\n"})
	entries := []SweepEntry{{Repo: "one", Checks: Run(dir, []string{"rules.yaml"}, "")}, {Repo: "two", Checks: Run(dir, []string{"rules.yaml"}, "")}}
	ev := SweepEvidence(testRevision, strings.Repeat("b", 64), entries, time.Now())
	doc, _ := json.Marshal(ev)
	got, err := attest.Verify(doc)
	if err != nil || got.Result != "pass" || got.Counts.Pass != 2 {
		t.Fatalf("result %s counts %+v err %v", got.Result, got.Counts, err)
	}
}
