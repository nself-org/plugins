package provenance_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/provenance"
)

func TestCheckStrict(t *testing.T) {
	deep := strings.Repeat("[", provenance.MaxDepth+1) + strings.Repeat("]", provenance.MaxDepth+1)
	atLimit := strings.Repeat("[", provenance.MaxDepth) + strings.Repeat("]", provenance.MaxDepth)
	for doc, ok := range map[string]bool{
		`{}`:                        true,
		`[]`:                        true,
		`"s"`:                       true,
		`1`:                         true,
		`{"a":1,"b":{"a":2}}`:       true,
		`{"a":[{"b":1},{"b":2}]}`:   true,
		atLimit:                     true,
		deep:                        false,
		``:                          false,
		`{"a":1,"a":2}`:             false,
		`{"a":1,"A":2}`:             false,
		"{\"k\":1,\"K\":2}":         false, // Kelvin sign folds onto k
		`{"a":{"b":1,"b":2}}`:       false,
		`{"a":[{"b":1,"b":2}]}`:     false,
		`{} {}`:                     false,
		`{}x`:                       false,
		`{"a":`:                     false,
		"\xff":                      false,
		`{"a":"b" "c":1}`:           false,
		`{"a":1}{"a":1}`:            false,
		`[1,2,{"x":1}]`:             true,
		`{"a":null,"b":true}`:       true,
		`{"a":{"b":{"c":1}},"c":1}`: true,
	} {
		err := provenance.CheckStrict([]byte(doc))
		if ok && err != nil {
			t.Errorf("%q refused: %v", doc, err)
		}
		if !ok && !errors.Is(err, provenance.ErrInvalid) {
			t.Errorf("%q accepted or wrong error: %v", doc, err)
		}
	}
}
