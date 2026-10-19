package providers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

type statusFake struct{}

func (statusFake) Post(context.Context, report.Status) error { return nil }

// TestStatusReporterIsReportReporter: StatusReporter is an alias, so a value
// of either type is the other (one interface, Constitution §3.4).
func TestStatusReporterIsReportReporter(t *testing.T) {
	var s StatusReporter = statusFake{}
	var r report.Reporter = s
	if r == nil {
		t.Fatal("alias")
	}
}

func TestStatusReportOwnerKey(t *testing.T) {
	if errs := StatusRegistrationErrors(); len(errs) != 0 {
		t.Fatal(errs)
	}
	spec, ok := policy.Default.Find(KeyReportOwner)
	if !ok || !spec.Spend || spec.Default != nil {
		t.Fatalf("key spec: %+v %v", spec, ok)
	}
	for in, want := range map[string]bool{`"team"`: true, `"personal"`: true, `"both"`: false, `""`: false, `true`: false, `["team"]`: false} {
		if _, err := spec.Decode(json.RawMessage(in)); (err == nil) != want {
			t.Fatalf("Decode(%s) error = %v, want ok=%v", in, err, want)
		}
	}
	if got := ReportOwner(policy.Effective{}); got != report.OwnerPersonal {
		t.Fatalf("a missing value is personal, got %q", got)
	}
}

// TestStatusReportOwnerMerges: a real policy merge accepts "team" from the
// operator scopes (the Fixed kind with a default would refuse it, E670), a
// missing value is personal, and disagreeing scopes still conflict.
func TestStatusReportOwnerMerges(t *testing.T) {
	reg, err := trust.NewRegistry(ReportOwnerKey())
	if err != nil {
		t.Fatal(err)
	}
	set := func(kind trust.Source, v string) trust.Scope {
		return trust.Scope{Kind: kind, Values: map[string]json.RawMessage{KeyReportOwner: json.RawMessage(v)}}
	}
	for _, kind := range []trust.Source{trust.SourceUser, trust.SourceTeam, trust.SourceProject} {
		eff, _, err := reg.Merge("", set(kind, `"team"`))
		if err != nil || ReportOwner(eff) != report.OwnerTeam {
			t.Fatalf("%s scope: %v %q", kind, err, ReportOwner(eff))
		}
	}
	if eff, _, err := reg.Merge(""); err != nil || ReportOwner(eff) != report.OwnerPersonal {
		t.Fatalf("missing value: %v %q", err, ReportOwner(eff))
	}
	if _, _, err := reg.Merge("", set(trust.SourceTeam, `"team"`), set(trust.SourceProject, `"personal"`)); err == nil {
		t.Fatal("a lower scope must not change a fixed value")
	}
	if _, _, err := reg.Merge("", set(trust.SourceUser, `"both"`)); err == nil {
		t.Fatal("an unknown value is refused")
	}
}
