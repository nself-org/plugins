package trust

// Purpose: the run scope (D-0984): per-run flag values reach the effective
// policy with operator authority, tighten-only except for preferences.

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func runScopeRegistry(t *testing.T) Registry {
	t.Helper()
	atoi := func(raw json.RawMessage) (any, error) {
		var n int64
		if err := json.Unmarshal(raw, &n); err != nil {
			return nil, err
		}
		return n, nil
	}
	r, err := NewRegistry(
		KeySpec{Key: "sched.preset", Owner: "sched", Kind: TightenAny, Default: "balanced"},
		KeySpec{Key: "sched.overrides.runner", Owner: "sched", Kind: TightenFixed},
		KeySpec{Key: "sched.max_queue", Owner: "sched", Kind: TightenLower, Default: int64(math.MaxInt64), Decode: atoi},
		KeySpec{Key: "sched.max_cost_micro", Owner: "sched", Kind: TightenLower, Default: int64(1000), Spend: true, Decode: atoi},
	)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func runValues(kv ...string) Scope {
	v := map[string]json.RawMessage{}
	for i := 0; i < len(kv); i += 2 {
		v[kv[i]] = json.RawMessage(kv[i+1])
	}
	return Scope{Kind: SourceRun, Values: v}
}

func TestRunScopeAppliesPreferenceRunnerAndQueue(t *testing.T) {
	r := runScopeRegistry(t)
	e, d0, err := r.Merge("")
	if err != nil {
		t.Fatal(err)
	}
	_ = e
	e, d1, err := r.Merge("", runValues("sched.preset", `"prefer-local"`, "sched.overrides.runner", `"ms5"`, "sched.max_queue", `600000`))
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]any{"sched.preset": "prefer-local", "sched.overrides.runner": "ms5", "sched.max_queue": int64(600000)} {
		got, src, ok := e.Value(key)
		if !ok || got != want || src != SourceRun {
			t.Errorf("%s = %v from %s (ok %v), want %v from run", key, got, src, ok, want)
		}
	}
	if len(e.Ignored()) != 0 {
		t.Errorf("ignored: %v", e.Ignored())
	}
	if d0 == d1 {
		t.Error("the digest does not bind the run values")
	}
}

func TestRunScopeIsTightenOnlyAgainstOperatorScopes(t *testing.T) {
	r := runScopeRegistry(t)
	project := Scope{Kind: SourceProject, Values: map[string]json.RawMessage{"sched.overrides.runner": json.RawMessage(`"mm4"`), "sched.max_queue": json.RawMessage(`300000`)}}
	e, _, err := r.Merge("", project, runValues("sched.overrides.runner", `"ms5"`, "sched.max_queue", `600000`, "sched.preset", `"prefer-local"`))
	if err != nil {
		t.Fatalf("a run flag that loosens must be ignored, not fail the run: %v", err)
	}
	if v, src, _ := e.Value("sched.overrides.runner"); v != "mm4" || src != SourceProject {
		t.Errorf("runner = %v from %s", v, src)
	}
	if v, src, _ := e.Value("sched.max_queue"); v != int64(300000) || src != SourceProject {
		t.Errorf("max_queue = %v from %s", v, src)
	}
	if v, src, _ := e.Value("sched.preset"); v != "prefer-local" || src != SourceRun {
		t.Errorf("preset (a preference) = %v from %s", v, src)
	}
	ignored := map[string]bool{}
	for _, ig := range e.Ignored() {
		if ig.Scope != SourceRun || !strings.Contains(ig.Reason, "E671") {
			t.Errorf("ignored entry %+v", ig)
		}
		ignored[ig.Key] = true
	}
	if !ignored["sched.overrides.runner"] || !ignored["sched.max_queue"] || len(ignored) != 2 {
		t.Errorf("ignored keys: %v", ignored)
	}
}

func TestRunScopeHasOperatorAuthorityOverSpendKeys(t *testing.T) {
	r := runScopeRegistry(t)
	rev := Scope{Kind: SourceRevision, Values: map[string]json.RawMessage{"sched.max_cost_micro": json.RawMessage(`10`)}}
	e, _, _ := r.Merge("", rev)
	if len(e.Ignored()) != 1 {
		t.Fatalf("a revision must not set a spend key: %v", e.Ignored())
	}
	e, _, err := r.Merge("", runValues("sched.max_cost_micro", `10`))
	if err != nil {
		t.Fatal(err)
	}
	if v, src, _ := e.Value("sched.max_cost_micro"); v != int64(10) || src != SourceRun || len(e.Ignored()) != 0 {
		t.Errorf("run spend key = %v from %s, ignored %v", v, src, e.Ignored())
	}
}

func TestRunScopeAppliesLast(t *testing.T) {
	r := runScopeRegistry(t)
	rev := Scope{Kind: SourceRevision, Values: map[string]json.RawMessage{"sched.max_queue": json.RawMessage(`900000`)}}
	e, _, err := r.Merge("", runValues("sched.max_queue", `600000`), rev)
	if err != nil {
		t.Fatal(err)
	}
	if v, src, _ := e.Value("sched.max_queue"); v != int64(600000) || src != SourceRun {
		t.Errorf("max_queue = %v from %s", v, src)
	}
}
