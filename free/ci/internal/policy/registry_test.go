package policy

import (
	"encoding/json"
	"strings"
	"testing"
)

func okDecode(raw json.RawMessage) (any, error) { return decodeOne(raw) }

func TestRegisterKeyRefusals(t *testing.T) {
	k := NewKeyset()
	good := KeySpec{Key: "sched.*", Owner: "sched", Kind: TightenAny, Decode: okDecode}
	if err := k.Register(good); err != nil {
		t.Fatal(err)
	}
	cases := map[string]KeySpec{
		"duplicate":            good,
		"overlapping family":   {Key: "sched.fast", Owner: "other", Kind: TightenAny, Decode: okDecode},
		"overlapping reverse":  {Key: "sched.*.x", Owner: "other", Kind: TightenAny, Decode: okDecode},
		"nil decode":           {Key: "a.b", Owner: "x", Kind: TightenAny},
		"no owner":             {Key: "a.c", Decode: okDecode},
		"no key":               {Owner: "x", Decode: okDecode},
		"lower without order":  {Key: "a.d", Owner: "x", Kind: TightenLower, Decode: okDecode},
		"default not in order": {Key: "a.e", Owner: "x", Kind: TightenHigher, Order: []string{"a", "b"}, Default: "c", Decode: okDecode},
		"default wrong type":   {Key: "a.f", Owner: "x", Kind: TightenLower, Order: []string{"a", "b"}, Default: 3, Decode: okDecode},
		"subset non-list":      {Key: "a.g", Owner: "x", Kind: TightenSubset, Default: "a", Decode: okDecode},
		"only-on non-bool":     {Key: "a.h", Owner: "x", Kind: TightenOnlyOn, Default: "yes", Decode: okDecode},
		"unknown kind":         {Key: "a.i", Owner: "x", Kind: Tightening(99), Decode: okDecode},
		"double star":          {Key: "a.*.*", Owner: "x", Kind: TightenAny, Decode: okDecode},
	}
	for name, spec := range cases {
		err := k.Register(spec)
		if err == nil || !strings.Contains(err.Error(), "E672") && !strings.Contains(err.Error(), "invalid key spec") {
			t.Errorf("%s: %v", name, err)
		}
	}
	if len(k.Keys()) != 1 {
		t.Fatalf("a refused key was kept: %v", k.Keys())
	}
	if _, ok := k.Find("sched.fast"); !ok {
		t.Error("prefix.* must match one segment")
	}
	for _, miss := range []string{"sched", "sched.a.b", "other.fast"} {
		if _, ok := k.Find(miss); ok {
			t.Errorf("%s matched sched.*", miss)
		}
	}
	if err := RegisterKey(KeySpec{Key: "trust.network", Owner: "x", Kind: TightenAny, Decode: okDecode}); err == nil {
		t.Error("the process-wide set accepted a duplicate trust key")
	}
}

func TestTrustKeysRegistered(t *testing.T) {
	for _, spec := range trustKeys() {
		if _, ok := Default.Find(spec.Key); !ok {
			t.Errorf("%s not registered", spec.Key)
		}
		if spec.Spend {
			t.Errorf("%s must not be a Spend key", spec.Key)
		}
	}
}

func TestFlatten(t *testing.T) {
	got, err := flatten(map[string]any{"trust": map[string]any{"approval": map[string]any{"first_time": true}, "network": "none"}, "sched.fast": json.Number("3")})
	if err != nil {
		t.Fatal(err)
	}
	if string(got["trust.approval.first_time"]) != "true" || string(got["trust.network"]) != `"none"` || string(got["sched.fast"]) != "3" || len(got) != 3 {
		t.Fatalf("flatten: %v", got)
	}
	for _, bad := range []map[string]any{{"": 1}, {"a..b": 1}, {"a": map[string]any{"b": nil}}, {"a.b": 1, "a": map[string]any{"b": 2}}} {
		if _, err := flatten(bad); err == nil {
			t.Errorf("accepted %v", bad)
		}
	}
	if got, err := flatten(nil); got != nil || err != nil {
		t.Fatalf("empty: %v %v", got, err)
	}
}
