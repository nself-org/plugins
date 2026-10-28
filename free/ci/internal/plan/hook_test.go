package plan

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestPlacePlanHook covers the cmd placePlan contract: nil adds nothing;
// a registered hook adds data.placement and the human column and leaves
// every other byte of the document unchanged.
func TestPlacePlanHook(t *testing.T) {
	ctx := context.Background()
	base := mustBuild(t, matrixInput(t, "xcode"))
	plain := envelope(t, base)
	want, err := os.ReadFile(filepath.Join("testdata", "matrix.golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(plain) != string(want) || strings.Contains(string(plain), `"placement"`) {
		t.Fatal("nil hook: document differs from the golden or has a placement key")
	}
	if cells, err := base.ApplyPlacement(ctx, nil); cells != nil || err != nil {
		t.Fatalf("nil hook returned %v %v", cells, err)
	}

	var seen *Plan
	fake := func(_ context.Context, p *Plan) (any, []string, error) {
		seen = p
		cells := make([]string, len(p.Jobs))
		for i := range cells {
			cells[i] = "local"
		}
		return map[string]any{"runner": "local", "jobs": len(p.Jobs)}, cells, nil
	}
	placed := mustBuild(t, matrixInput(t, "xcode"))
	cells, err := placed.ApplyPlacement(ctx, fake)
	if err != nil {
		t.Fatal(err)
	}
	if seen != placed || len(seen.Jobs) != len(placed.Jobs) || seen.Selection != placed.Selection {
		t.Fatalf("hook saw %+v", seen)
	}
	got := envelope(t, placed)
	tail := "\n  }\n}\n"
	if !strings.HasPrefix(string(got), strings.TrimSuffix(string(plain), tail)+",\n    \"placement\": {") {
		t.Fatalf("placed document does not extend the plain one byte for byte:\n%s", got)
	}
	var a, b map[string]any
	if json.Unmarshal(plain, &a) != nil || json.Unmarshal(got, &b) != nil {
		t.Fatal("documents are not JSON")
	}
	data := b["data"].(map[string]any)
	if data["placement"].(map[string]any)["runner"] != "local" {
		t.Errorf("placement = %v", data["placement"])
	}
	delete(data, "placement")
	if !reflect.DeepEqual(a, b) {
		t.Error("keys other than placement changed")
	}
	table := placed.Human(cells)
	if !strings.HasSuffix(table[1], "PLACEMENT") || !strings.HasSuffix(table[2], "local") {
		t.Errorf("human table lacks the placement column:\n%s", strings.Join(table, "\n"))
	}

	boom := errors.New("boom")
	failing := func(context.Context, *Plan) (any, []string, error) { return nil, nil, boom }
	if _, err := mustBuild(t, matrixInput(t, "xcode")).ApplyPlacement(ctx, failing); !errors.Is(err, boom) {
		t.Errorf("hook error not returned: %v", err)
	}
	// A hook that returns no data adds no key.
	empty := func(context.Context, *Plan) (any, []string, error) { return nil, nil, nil }
	q := mustBuild(t, matrixInput(t, "xcode"))
	if _, err := q.ApplyPlacement(ctx, empty); err != nil || strings.Contains(string(envelope(t, q)), `"placement"`) {
		t.Error("nil placement data must add no key")
	}
}
