package planview

// Purpose: the tier_rank score component added by P7-AUTO-27 (World.Bias) stays
// valid placement-plan v1 and shows in the human explain lines.

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
)

func TestTierRankComponentIsValidV1(t *testing.T) {
	w, jobs, _ := fixture(t)
	w.Bias = map[string]int64{"local": 7000}
	res := sched.Place(w, jobs)
	doc := Build(Input{Name: "default", Revision: strings.Repeat("a", 40), Trigger: "local", PolicyDigest: "p" + strings.Repeat("0", 63), WorldDigest: "w" + strings.Repeat("0", 63), World: w, Jobs: jobs, Result: res})
	schema, err := os.ReadFile("../../schemas/placement-plan.v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(doc)
	if !strings.Contains(string(raw), `"tier_rank"`) {
		t.Fatalf("the biased runner must show a tier_rank component: %s", raw)
	}
	if err = model.ValidateJSON(schema, raw); err != nil {
		t.Fatalf("a document with tier_rank must stay valid v1: %v", err)
	}
	lines := strings.Join(ExplainLines(doc, SourcePlan), "\n")
	if !strings.Contains(lines, "tier_rank=7000") {
		t.Fatalf("explain lines must show tier_rank:\n%s", lines)
	}
	// without a bias the nine components are all there is
	w.Bias = nil
	plain, _ := json.Marshal(Build(Input{Name: "default", Revision: strings.Repeat("a", 40), Trigger: "local", PolicyDigest: "p" + strings.Repeat("0", 63), WorldDigest: "w" + strings.Repeat("0", 63), World: w, Jobs: jobs, Result: sched.Place(w, jobs)}))
	if strings.Contains(string(plain), "tier_rank") {
		t.Fatal("tier_rank must not appear without a bias")
	}
}
