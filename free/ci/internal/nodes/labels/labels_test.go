package labels

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestParseValid(t *testing.T) {
	got, err := Parse([]string{"zone=eu-1", "gpu", "arch.v2=Arm_64", "e="})
	if err != nil {
		t.Fatal(err)
	}
	want := []store.NodeLabel{{Key: "arch.v2", Value: "Arm_64"}, {Key: "e"}, {Key: "gpu"}, {Key: "zone", Value: "eu-1"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if s := Strings(got); !reflect.DeepEqual(s, []string{"arch.v2=Arm_64", "e", "gpu", "zone=eu-1"}) {
		t.Fatalf("strings %v", s)
	}
}

func TestValidateReasons(t *testing.T) {
	long := "a" + strings.Repeat("b", 63)
	cases := []struct {
		in     []string
		reason string
	}{
		{[]string{"Gpu"}, ReasonSyntax},
		{[]string{"1gpu"}, ReasonSyntax},
		{[]string{""}, ReasonSyntax},
		{[]string{"=x"}, ReasonSyntax},
		{[]string{long}, ReasonSyntax},
		{[]string{"k=" + strings.Repeat("v", 64)}, ReasonSyntax},
		{[]string{"k=a b"}, ReasonSyntax},
		{[]string{"k=a=b"}, ReasonSyntax},
		{[]string{"k=1", "k=2"}, ReasonSyntax},
		{[]string{"nself.visibility=public"}, ReasonReserved},
		{[]string{"nself."}, ReasonReserved},
	}
	for _, c := range cases {
		_, err := Parse(c.in)
		var e *Error
		if !errors.As(err, &e) || e.Code != CodeLabel || ReasonOf(err) != c.reason {
			t.Fatalf("%v: got %v want %s", c.in, err, c.reason)
		}
	}
	if _, err := Parse([]string{strings.Repeat("a", 63), "k=" + strings.Repeat("v", 63), "nself"}); err != nil {
		t.Fatalf("boundary set rejected: %v", err)
	}
	if ReasonOf(errors.New("other")) != "" {
		t.Fatal("foreign error has a reason")
	}
}

func TestValidateCount(t *testing.T) {
	set := make([]string, MaxLabels)
	for i := range set {
		set[i] = fmt.Sprintf("k%d", i)
	}
	if _, err := Parse(set); err != nil {
		t.Fatalf("%d labels rejected: %v", MaxLabels, err)
	}
	if _, err := Parse(append(set, "extra")); ReasonOf(err) != ReasonTooMany {
		t.Fatalf("too many: %v", err)
	}
}

func TestCodeRegistered(t *testing.T) {
	for _, p := range model.CodeProblems() {
		if strings.Contains(p, CodeLabel) {
			t.Fatalf("code problem: %s", p)
		}
	}
}

type fakeReader struct {
	set []store.NodeLabel
	err error
}

func (f fakeReader) NodeLabels(context.Context, string) ([]store.NodeLabel, error) {
	return f.set, f.err
}

func TestSourceFromStore(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	set, err := Parse([]string{"pool=gpu", "fast"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.NodeLabelsReplace(ctx, "node-a", set, "operator"); err != nil {
		t.Fatal(err)
	}
	got, err := Source(s)(ctx, "node-a")
	if err != nil || !reflect.DeepEqual(got, []string{"fast", "pool=gpu"}) {
		t.Fatalf("got %v %v", got, err)
	}
	if got, err = Source(s)(ctx, "node-b"); err != nil || len(got) != 0 {
		t.Fatalf("unlabelled node: %v %v", got, err)
	}
	if _, err = Source(fakeReader{err: errors.New("disk")})(ctx, "x"); err == nil {
		t.Fatal("read error swallowed")
	}
}

func TestRegisterSourceOnce(t *testing.T) {
	if err := RegisterSource(fakeReader{}); err != nil {
		t.Fatal(err)
	}
	if err := RegisterSource(fakeReader{}); err == nil {
		t.Fatal("second label source accepted")
	}
}
