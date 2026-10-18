package policy

import (
	"math"
	"strings"
	"testing"
)

func TestRegisterNumericKey(t *testing.T) {
	k := NewKeyset()
	good := KeySpec{Key: "n.cap", Owner: "x", Kind: TightenLower, Default: int64(math.MaxInt64), Decode: okDecode}
	if err := k.Register(good); err != nil {
		t.Fatalf("numeric Lower key refused: %v", err)
	}
	if err := k.Register(KeySpec{Key: "n.floor", Owner: "x", Kind: TightenHigher, Default: 3, Decode: okDecode}); err != nil {
		t.Fatalf("numeric Higher key refused: %v", err)
	}
	for name, spec := range map[string]KeySpec{
		"no order, no default":   {Key: "n.a", Owner: "x", Kind: TightenLower, Decode: okDecode},
		"no order, text default": {Key: "n.b", Owner: "x", Kind: TightenHigher, Default: "5", Decode: okDecode},
		"no order, float":        {Key: "n.c", Owner: "x", Kind: TightenLower, Default: 1.5, Decode: okDecode},
	} {
		if err := k.Register(spec); err == nil || !strings.Contains(err.Error(), "E672") {
			t.Errorf("%s: %v", name, err)
		}
	}
	if len(k.Keys()) != 2 {
		t.Fatalf("a refused numeric key was kept: %v", k.Keys())
	}
}
