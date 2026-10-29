package run

import (
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/report"
)

// Exercise the real attest publisher through finish/postStatus and the outbox.
func TestAttestPublisherDeliveredStatus(t *testing.T) {
	for _, tc := range []struct {
		name, script string
		state        report.State
	}{
		{"pass", "true", report.Success}, {"fail", "exit 1", report.Failure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t, map[string]model.Job{"test": sh(tc.script)}, gitFake{})
			r.C.Hooks.Publisher = attestPublisher{}
			res := r.run(t)
			posted := r.Rep.all()
			if len(posted) != 1 {
				t.Fatalf("posted %+v", posted)
			}
			s := posted[0]
			if s.State != tc.state || s.TargetURL != "" || !strings.HasPrefix(s.Description, tc.name+":") || !strings.Contains(s.Description, "evidence "+res.Summary.Evidence[:12]) {
				t.Fatalf("delivered status: %+v", s)
			}
		})
	}
}
