package trust_test

// Purpose: the per-run scope of trust.Merge (P7-AUTO-29, D-0984).
// Constraints: seeded generators print the seed on failure; no clocks, no I/O.

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/trust"
)

const runPreset = "sched.preset"

// runRegistry has one key of every tightening kind, with and without a default.
func runRegistry(t *testing.T) trust.Registry {
	t.Helper()
	ord := []string{"a", "b", "c"}
	reg, err := trust.NewRegistry(
		trust.KeySpec{Key: runPreset, Owner: "x", Kind: trust.TightenAny, Default: "balanced"},
		trust.KeySpec{Key: "x.any", Owner: "x", Kind: trust.TightenAny, Default: "a"},
		trust.KeySpec{Key: "x.anynone", Owner: "x", Kind: trust.TightenAny},
		trust.KeySpec{Key: "x.low", Owner: "x", Kind: trust.TightenLower, Default: int64(math.MaxInt64), Decode: numDecode},
		trust.KeySpec{Key: "x.lownone", Owner: "sched", Kind: trust.TightenLower, Decode: numDecode},
		trust.KeySpec{Key: "x.high", Owner: "x", Kind: trust.TightenHigher, Default: int64(1), Decode: numDecode},
		trust.KeySpec{Key: "x.ordlow", Owner: "x", Kind: trust.TightenLower, Order: ord, Default: "c"},
		trust.KeySpec{Key: "x.ordhigh", Owner: "x", Kind: trust.TightenHigher, Order: ord, Default: "a"},
		trust.KeySpec{Key: "x.fixed", Owner: "sched", Kind: trust.TightenFixed},
		trust.KeySpec{Key: "x.fixeddef", Owner: "x", Kind: trust.TightenFixed, Default: "p"},
		trust.KeySpec{Key: "x.on", Owner: "x", Kind: trust.TightenOnlyOn, Default: false},
		trust.KeySpec{Key: "x.onnone", Owner: "sched", Kind: trust.TightenOnlyOn},
		trust.KeySpec{Key: "x.sub", Owner: "sched", Kind: trust.TightenSubset},
		trust.KeySpec{Key: "x.subdef", Owner: "x", Kind: trust.TightenSubset, Default: []string{"a", "b", "c"}},
		trust.KeySpec{Key: "x.sup", Owner: "sched", Kind: trust.TightenSuperset},
		trust.KeySpec{Key: "x.fam.*", Owner: "x", Kind: trust.TightenLower, Default: int64(100), Decode: numDecode},
		// an unset key of a non-placement owner can mean deny-all (trust.providers): a run never fills it
		trust.KeySpec{Key: "x.subclosed", Owner: "trust", Kind: trust.TightenSubset},
		trust.KeySpec{Key: "x.fixedclosed", Owner: "trust", Kind: trust.TightenFixed},
		trust.KeySpec{Key: "x.lowclosed", Owner: "trust", Kind: trust.TightenLower, Decode: numDecode},
		trust.KeySpec{Key: "x.onclosed", Owner: "trust", Kind: trust.TightenOnlyOn},
		trust.KeySpec{Key: "x.secret", Owner: "x", Kind: trust.TightenFixed, Secret: true},
		trust.KeySpec{Key: "x.spend", Owner: "x", Kind: trust.TightenLower, Default: int64(5), Spend: true, Decode: numDecode},
	)
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

// runDomain is the JSON values each key is generated from.
var runDomain = map[string][]string{
	runPreset: {`"balanced"`, `"local"`, `"auto"`}, "x.any": {`"a"`, `"b"`}, "x.anynone": {`"a"`, `"b"`},
	"x.low": {`1`, `2`, `4`, `100`}, "x.lownone": {`1`, `3`, `6`}, "x.high": {`1`, `2`, `5`},
	"x.ordlow": {`"a"`, `"b"`, `"c"`}, "x.ordhigh": {`"a"`, `"b"`, `"c"`},
	"x.fixed": {`"p"`, `"q"`}, "x.fixeddef": {`"p"`, `"q"`}, "x.on": {`true`, `false`}, "x.onnone": {`true`, `false`},
	"x.sub": {`[]`, `["a"]`, `["a","b"]`, `["b","c"]`, `["a","b","c"]`}, "x.subdef": {`[]`, `["a"]`, `["a","b"]`, `["b","c"]`, `["a","b","c","d"]`},
	"x.sup": {`[]`, `["a"]`, `["a","b"]`, `["a","b","c"]`}, "x.fam.a": {`1`, `50`, `100`, `200`}, "x.spend": {`1`, `5`, `9`}, "x.secret": {`"s1"`, `"s2"`},
	"x.subclosed": {`[]`, `["a"]`, `["a","b"]`}, "x.fixedclosed": {`"p"`}, "x.lowclosed": {`1`, `3`}, "x.onclosed": {`true`},
}

func randomValues(r *rand.Rand, keys []string) map[string]json.RawMessage {
	out := map[string]json.RawMessage{}
	for _, k := range keys {
		if r.Intn(3) == 0 {
			d := runDomain[k]
			out[k] = json.RawMessage(d[r.Intn(len(d))])
		}
	}
	return out
}

func sortedKeys() []string {
	keys := make([]string, 0, len(runDomain))
	for k := range runDomain {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func setOf(v any) map[string]bool {
	out := map[string]bool{}
	switch xs := v.(type) {
	case []string:
		for _, x := range xs {
			out[x] = true
		}
	case []any:
		for _, x := range xs {
			out[fmt.Sprint(x)] = true
		}
	}
	return out
}

func subset(a, b map[string]bool) bool {
	for x := range a {
		if !b[x] {
			return false
		}
	}
	return true
}

// looser is an oracle written without trust's own stricter(): it says whether
// next, with the run scope, is looser than before, without it. present is
// whether the baseline holds the key.
func looser(key string, spec trust.KeySpec, before any, present bool, after any) bool {
	if key == runPreset {
		return false
	}
	if spec.Spend {
		return !reflect.DeepEqual(before, after)
	}
	switch spec.Kind {
	case trust.TightenAny:
		return !reflect.DeepEqual(before, after)
	case trust.TightenFixed:
		return present && !reflect.DeepEqual(before, after)
	case trust.TightenOnlyOn:
		return present && before == true && after != true
	case trust.TightenLower, trust.TightenHigher:
		if !present {
			return false
		}
		if len(spec.Order) > 0 {
			x, y := slices.Index(spec.Order, before.(string)), slices.Index(spec.Order, after.(string))
			return spec.Kind == trust.TightenLower && y > x || spec.Kind == trust.TightenHigher && y < x
		}
		x, _ := trust.IntValue(before)
		y, _ := trust.IntValue(after)
		return spec.Kind == trust.TightenLower && y > x || spec.Kind == trust.TightenHigher && y < x
	case trust.TightenSubset:
		return present && !subset(setOf(after), setOf(before))
	case trust.TightenSuperset:
		return present && !subset(setOf(before), setOf(after))
	}
	return true
}

// TestRunScopeNeverLoosens: over 10 000 generated scope stacks the effective
// policy with run-scope values is never looser than without them (sched.preset
// aside), the run scope never sets a Spend key, and it never errors.
func TestRunScopeNeverLoosens(t *testing.T) {
	reg := runRegistry(t)
	keys := sortedKeys()
	seed := int64(20261019)
	r := rand.New(rand.NewSource(seed))
	kinds := []trust.Source{trust.SourceUser, trust.SourceTeam, trust.SourceProject, trust.SourceProtectedBranch, trust.SourceRevision}
	valid, withIgnored := 0, 0
	for attempts := 0; valid < 10000 && attempts < 400000; attempts++ {
		var stack []trust.Scope
		for _, k := range kinds {
			if r.Intn(2) == 0 {
				stack = append(stack, trust.Scope{Kind: k, Values: randomValues(r, keys)})
			}
		}
		base, baseDigest, err := reg.Merge("", stack...)
		if err != nil {
			continue // a lower scope loosened another: not a stack this test is about
		}
		valid++
		run := trust.Scope{Kind: trust.SourceRun, Values: randomValues(r, keys)}
		got, digest, err := reg.Merge("", append(append([]trust.Scope(nil), stack...), run)...)
		if err != nil {
			t.Fatalf("seed %d: run scope errored: %v (stack %v run %v)", seed, err, stack, run.Values)
		}
		if len(run.Values) == 0 && digest != baseDigest {
			t.Fatalf("seed %d: empty run scope changed the digest", seed)
		}
		for _, k := range keys {
			spec, _ := reg.Find(k)
			before, _, present := base.Value(k)
			inBase := present
			if !present && spec.Default != nil && strings.HasSuffix(spec.Key, ".*") {
				before, present = spec.Default, true // a family key has no seeded default: the default is what holds
			}
			after, src, ok := got.Value(k)
			if inBase && !ok {
				t.Fatalf("seed %d: %s vanished", seed, k)
			}
			if !ok {
				continue
			}
			if (spec.Spend || spec.Secret) && src == trust.SourceRun {
				t.Fatalf("seed %d: spend or secret key %s set by the run scope", seed, k)
			}
			if spec.Default == nil && spec.Owner != "sched" && !inBase && src == trust.SourceRun && k != runPreset {
				t.Fatalf("seed %d: the run scope filled the unset %s key %s", seed, spec.Owner, k)
			}
			if !inBase && src != trust.SourceRun {
				t.Fatalf("seed %d: %s appeared from %s", seed, k, src)
			}
			if looser(k, spec, before, present, after) {
				t.Fatalf("seed %d: %s loosened %v -> %v by the run scope\nstack %v\nrun %v", seed, k, before, after, stack, run.Values)
			}
		}
		if len(got.Ignored()) > len(base.Ignored()) {
			withIgnored++
		}
	}
	if valid < 10000 {
		t.Fatalf("seed %d: only %d valid stacks generated", seed, valid)
	}
	if withIgnored < 1000 {
		t.Fatalf("seed %d: the generator hit the ignore path only %d times", seed, withIgnored)
	}
}

// TestRunScopePreferenceOnlyPreset: sched.preset takes any value of the run
// scope; every other TightenAny key keeps its value and reports the attempt.
func TestRunScopePreferenceOnlyPreset(t *testing.T) {
	reg := runRegistry(t)
	project := trust.Scope{Kind: trust.SourceProject, Values: map[string]json.RawMessage{runPreset: json.RawMessage(`"auto"`), "x.any": json.RawMessage(`"a"`)}}
	run := trust.Scope{Kind: trust.SourceRun, Values: map[string]json.RawMessage{runPreset: json.RawMessage(`"local"`), "x.any": json.RawMessage(`"b"`), "x.anynone": json.RawMessage(`"b"`)}}
	eff, _, err := reg.Merge("", project, run)
	if err != nil {
		t.Fatal(err)
	}
	if v, src, _ := eff.Value(runPreset); v != "local" || src != trust.SourceRun {
		t.Errorf("preset = %v from %s, want local from run", v, src)
	}
	if v, src, _ := eff.Value("x.any"); v != "a" || src != trust.SourceProject {
		t.Errorf("x.any = %v from %s, want the project's a", v, src)
	}
	if _, _, ok := eff.Value("x.anynone"); ok {
		t.Error("a free-form key without a default was set by the run scope")
	}
	if got := ignoredKeys(eff); len(got) != 2 || got["x.any"] == "" || got["x.anynone"] == "" {
		t.Errorf("ignored = %v", got)
	}
}

// TestRunScopeIgnoredReported: a pinned runner beats a different one, an
// unset Fixed key takes the run's value, a lower cap applies and a higher one
// is ignored, and a Spend or Secret key is refused with E819; each refusal is listed.
func TestRunScopeIgnoredReported(t *testing.T) {
	reg := runRegistry(t)
	project := trust.Scope{Kind: trust.SourceProject, Values: map[string]json.RawMessage{"x.fixeddef": json.RawMessage(`"p"`), "x.low": json.RawMessage(`8`)}}
	run := trust.Scope{Kind: trust.SourceRun, Values: map[string]json.RawMessage{
		"x.fixeddef": json.RawMessage(`"q"`), "x.fixed": json.RawMessage(`"ms5"`), "x.low": json.RawMessage(`9`), "x.lownone": json.RawMessage(`3`), "x.spend": json.RawMessage(`1`), "x.secret": json.RawMessage(`"s1"`),
	}}
	eff, _, err := reg.Merge("", project, run)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"x.fixeddef": "p", "x.fixed": "ms5", "x.low": int64(8), "x.lownone": int64(3), "x.spend": int64(5)}
	for k, w := range want {
		if v, _, _ := eff.Value(k); v != w {
			t.Errorf("%s = %v, want %v", k, v, w)
		}
	}
	if v, src, _ := eff.Value("x.fixed"); src != trust.SourceRun {
		t.Errorf("x.fixed = %v from %s", v, src)
	}
	got := ignoredKeys(eff)
	if _, _, set := eff.Value("x.secret"); set || len(got) != 4 || !strings.Contains(got["x.secret"], "secret") || !strings.Contains(got["x.fixeddef"], "E819") || !strings.Contains(got["x.low"], "E819") || !strings.Contains(got["x.spend"], "spend") {
		t.Errorf("ignored = %v", got)
	}
	for _, ig := range eff.Ignored() {
		if ig.Scope != trust.SourceRun {
			t.Errorf("%s ignored from %s", ig.Key, ig.Scope)
		}
	}
}

// TestRunScopeEmptyKeepsDigest: a run scope without values is the same policy.
func TestRunScopeEmptyKeepsDigest(t *testing.T) {
	reg := runRegistry(t)
	project := trust.Scope{Kind: trust.SourceProject, Values: map[string]json.RawMessage{"x.low": json.RawMessage(`8`)}}
	_, a, err := reg.Merge("pin", project)
	if err != nil {
		t.Fatal(err)
	}
	_, b, err := reg.Merge("pin", project, trust.Scope{Kind: trust.SourceRun})
	if err != nil || a != b {
		t.Fatalf("digest changed: %s vs %s (%v)", a, b, err)
	}
}

func ignoredKeys(eff trust.Effective) map[string]string {
	out := map[string]string{}
	for _, ig := range eff.Ignored() {
		out[ig.Key] = ig.Reason
	}
	return out
}
