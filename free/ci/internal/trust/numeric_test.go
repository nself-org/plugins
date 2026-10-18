package trust_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func numDecode(raw json.RawMessage) (any, error) {
	var n json.Number
	if err := json.Unmarshal(raw, &n); err != nil {
		return nil, err
	}
	i, err := n.Int64()
	return i, err
}

func TestIntValue(t *testing.T) {
	for _, tc := range []struct {
		in   any
		want int64
		ok   bool
	}{
		{int64(7), 7, true}, {3, 3, true}, {json.Number("12"), 12, true}, {json.Number("-4"), -4, true},
		{json.Number("1.5"), 0, false}, {json.Number("1e3"), 0, false}, {"7", 0, false}, {true, 0, false}, {nil, 0, false},
		{float64(2), 0, false}, {json.Number("9223372036854775808"), 0, false},
	} {
		got, ok := trust.IntValue(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("IntValue(%#v) = %d, %v", tc.in, got, ok)
		}
	}
}

// TestNumericTighten: a numeric Lower key lets a lower scope go lower and
// refuses to go higher; the revision scope ignores a widening and records it.
func TestNumericTighten(t *testing.T) {
	reg, err := trust.NewRegistry(
		trust.KeySpec{Key: "a.cap", Owner: "x", Kind: trust.TightenLower, Default: int64(math.MaxInt64), Decode: numDecode},
		trust.KeySpec{Key: "a.floor", Owner: "x", Kind: trust.TightenHigher, Default: int64(1), Decode: numDecode},
	)
	if err != nil {
		t.Fatal(err)
	}
	vals := func(cap, floor string) map[string]json.RawMessage {
		return map[string]json.RawMessage{"a.cap": json.RawMessage(cap), "a.floor": json.RawMessage(floor)}
	}
	project := trust.Scope{Kind: trust.SourceProject, Values: vals("8", "4")}
	eff, _, err := reg.Merge("", project, trust.Scope{Kind: trust.SourceRevision, Values: vals("5", "6")})
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]int64{"a.cap": 5, "a.floor": 6} {
		if v, src, _ := eff.Value(key); v != want || src != trust.SourceRevision {
			t.Errorf("%s = %v from %s, want %d from revision", key, v, src, want)
		}
	}
	eff, _, err = reg.Merge("", project, trust.Scope{Kind: trust.SourceRevision, Values: vals("16", "2")})
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]int64{"a.cap": 8, "a.floor": 4} {
		if v, src, _ := eff.Value(key); v != want || src != trust.SourceProject {
			t.Errorf("%s = %v from %s, want %d from project", key, v, src, want)
		}
	}
	if got := len(eff.Ignored()); got != 2 {
		t.Errorf("ignored %d, want 2: %v", got, eff.Ignored())
	}
	// A non-revision scope that loosens is an error (E670), not a quiet skip.
	if _, _, err = reg.Merge("", project, trust.Scope{Kind: trust.SourceProtectedBranch, Values: vals("16", "4")}); err == nil {
		t.Error("protected branch raised a cap and merge accepted it")
	}
	// Equal is not loosening.
	if _, _, err = reg.Merge("", project, trust.Scope{Kind: trust.SourceProtectedBranch, Values: vals("8", "4")}); err != nil {
		t.Errorf("equal value refused: %v", err)
	}
}

// TestNumericNonInteger: values that are not whole numbers are never stricter.
func TestNumericNonInteger(t *testing.T) {
	loose := func(raw json.RawMessage) (any, error) { return string(raw), nil }
	reg, err := trust.NewRegistry(trust.KeySpec{Key: "a.cap", Owner: "x", Kind: trust.TightenLower, Default: int64(10), Decode: loose})
	if err != nil {
		t.Fatal(err)
	}
	eff, _, err := reg.Merge("", trust.Scope{Kind: trust.SourceRevision, Values: map[string]json.RawMessage{"a.cap": json.RawMessage("5")}})
	if err != nil {
		t.Fatal(err)
	}
	if v, src, _ := eff.Value("a.cap"); v != int64(10) || src != trust.SourceDefault {
		t.Errorf("a string value tightened a numeric key: %v from %s", v, src)
	}
}
