package webhook

// Purpose: prove running hooks reach runner management regardless of payload tags.
// Inputs: job tags and runner identities. Outputs: exact errors and call counts.
// Constraints: no sockets; only invalid identities decline before the registrar.

import (
	"context"
	"errors"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/runners"
)

func TestRunningDeclinesBeforeRegistrar(t *testing.T) {
	for _, tc := range []struct {
		name, body, reason string
		wantCalls          int
	}{
		{"no tags", `{"build_id":1,"runner":{"id":42}}`, "", 1},
		{"foreign tags", `{"build_id":1,"runner":{"id":42},"tag_list":["linux"]}`, "", 1},
		{"runner tags only", `{"build_id":1,"runner":{"id":42,"tag_list":["nself"]}}`, "", 1},
		{"missing runner", `{"build_id":1,"tag_list":["nself"]}`, "running job without a job and runner", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			sentinel := errors.New("registrar unavailable")
			runners.UseRegistrar(func(context.Context) (*runners.Registrar, error) {
				calls++
				return nil, sentinel
			})
			t.Cleanup(func() { runners.UseRegistrar(nil) })
			p, err := parsePayload([]byte(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			_, err = (&Source{}).fromRunning(context.Background(), model.TriggerFacts{Repo: "acme/app"}, p)
			if tc.wantCalls == 0 {
				wantDecline(t, tc.name, err, reasonUnsupported, tc.reason)
			} else if !errors.Is(err, sentinel) {
				t.Fatalf("registrar error = %v, want %v", err, sentinel)
			}
			if calls != tc.wantCalls {
				t.Fatalf("registrar calls = %d, want %d", calls, tc.wantCalls)
			}
		})
	}
}
