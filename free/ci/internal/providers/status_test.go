package providers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/report"
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
	if !ok || !spec.Spend || spec.Default != "personal" {
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
