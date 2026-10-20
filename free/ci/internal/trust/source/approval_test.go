package source

// Purpose: tests of the approval binding rules (P7-TRUST-24, Epic D18): what voids an approval, what keeps
// it, who needs one, and the E688 registration.
// Inputs:  hand-built bindings; no store, no clock, no network.
// Outputs: pass or fail.
// Constraints: each case names the one field that changed; a binding keyed by the run id alone is never
//          valid (the mutants in testdata/mutants-approval drop each rule and must each fail a test here).

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func base() Binding {
	return Binding{Run: "run-1", HeadSHA: strings.Repeat("a", 40), ConfigDigest: "sha256:cfg", HeadRepo: "acme/app"}
}

func TestApprovalVoidOnSync(t *testing.T) {
	flip := func(f func(*Binding)) Binding { b := base(); f(&b); return b }
	cases := []struct {
		name string
		cur  Binding
		want []string
	}{
		{"same binding keeps the approval", base(), nil},
		{"re-run of the same SHA and digest keeps it", flip(func(b *Binding) { b.Run = "run-1" }), nil},
		{"synchronize: new head SHA", flip(func(b *Binding) { b.HeadSHA = strings.Repeat("b", 40) }), []string{VoidHeadSHA}},
		{"force-push to an older SHA", flip(func(b *Binding) { b.HeadSHA = strings.Repeat("0", 40) }), []string{VoidHeadSHA}},
		{"config digest changed", flip(func(b *Binding) { b.ConfigDigest = "sha256:other" }), []string{VoidConfigDigest}},
		{"head repo changed (fork swapped)", flip(func(b *Binding) { b.HeadRepo = "evil/app" }), []string{VoidHeadRepo}},
		{"head repo cleared", flip(func(b *Binding) { b.HeadRepo = "" }), []string{VoidHeadRepo}},
		{"another run", flip(func(b *Binding) { b.Run = "run-2" }), []string{VoidRun}},
		{"every field", Binding{Run: "r", HeadSHA: strings.Repeat("c", 40), ConfigDigest: "x", HeadRepo: "z/z"}, []string{VoidRun, VoidHeadSHA, VoidConfigDigest, VoidHeadRepo}},
		{"head repo case is not a change", flip(func(b *Binding) { b.HeadRepo = "ACME/App" }), nil},
		{"SHA hex case is not a change", flip(func(b *Binding) { b.HeadSHA = strings.ToUpper(b.HeadSHA) }), nil},
	}
	for _, c := range cases {
		got := VoidReasons(base(), c.cur)
		if !reflect.DeepEqual(got, c.want) && !(len(got) == 0 && len(c.want) == 0) {
			t.Errorf("%s: void reasons %v, want %v", c.name, got, c.want)
		}
		if Valid(base(), c.cur) != (len(c.want) == 0) {
			t.Errorf("%s: Valid disagrees with the reasons", c.name)
		}
	}
}

func TestApprovalBindingNeedsEveryAnchor(t *testing.T) {
	// An approval keyed by run id alone never validates, even against itself.
	for name, b := range map[string]Binding{
		"run only":         {Run: "run-1"},
		"no head SHA":      {Run: "run-1", ConfigDigest: "d"},
		"no config digest": {Run: "run-1", HeadSHA: "abc"},
		"no run":           {HeadSHA: "abc", ConfigDigest: "d"},
		"empty":            {},
	} {
		if Valid(b, b) {
			t.Errorf("%s: an incomplete binding must not be valid", name)
		}
		if got := VoidReasons(b, b); !reflect.DeepEqual(got, []string{VoidIncomplete}) {
			t.Errorf("%s: reasons %v", name, got)
		}
		if Valid(base(), b) || Valid(b, base()) {
			t.Errorf("%s: an incomplete side must void", name)
		}
	}
	local := Binding{Run: "r", HeadSHA: "abc", ConfigDigest: "d"} // a local run has no head repo
	if !Valid(local, local) {
		t.Error("an empty head repo on both sides is a valid binding")
	}
}

func TestApprovalRequiresApproval(t *testing.T) {
	cases := []struct {
		name     string
		recorded bool
		ok       bool
		class    model.TrustClass
		want     bool
	}{
		{"recorded yes beats an owner class", true, true, owner, true},
		{"recorded no beats an untrusted class", false, true, untrusted, false},
		{"unrecorded untrusted holds", false, false, untrusted, true},
		{"unrecorded collaborator runs", false, false, collaborator, false},
		{"unrecorded owner runs", false, false, owner, false},
		{"unrecorded unknown class decodes to untrusted", false, false, model.TrustClass("martian"), true},
		{"unrecorded empty class decodes to untrusted", false, false, model.TrustClass(""), true},
	}
	for _, c := range cases {
		if got := RequiresApproval(c.recorded, c.ok, c.class); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestApprovalCode(t *testing.T) {
	if problems := model.CodeProblems(); len(problems) != 0 {
		t.Fatalf("code registry: %v", problems)
	}
	err := NewApprovalError(ApprovalSelf, "the author cannot approve run %s", "run-1")
	if !strings.HasPrefix(err.Error(), "E688: ") || ApprovalReasonOf(err) != ApprovalSelf {
		t.Fatalf("E688 error: %v", err)
	}
	var ae *ApprovalError
	if !errors.As(err, &ae) || ae.ErrCode() != CodeApproval || ae.ErrReason() != ApprovalSelf {
		t.Fatalf("typed error: %v", err)
	}
	if ApprovalReasonOf(errors.New("plain")) != "" {
		t.Fatal("a plain error has no approval reason")
	}
}
