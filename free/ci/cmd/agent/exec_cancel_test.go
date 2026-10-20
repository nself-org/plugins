package main

import (
	"context"
	"strings"
	"testing"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestAgentCancellationFitsTransportResultDeadline(t *testing.T) {
	skipUnixOnly(t)
	for _, tc := range []struct{ name, script string }{
		{"cooperative", "echo cancel-ready; sleep 60 & wait"},
		{"ignores TERM", "trap '' TERM; echo cancel-ready; sleep 60 & wait"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			useState(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			started := make(chan struct{}, 1)
			done := make(chan struct{})
			var res ciexec.AttemptResult
			var err error
			go func() {
				defer close(done)
				res, _, err = execute(ctx, execSpec{AttemptID: "cancel-bound", CoordinatorID: "cancel-coord", Job: model.Job{Kind: "static", Isolation: "process", Run: []string{"sh", "-c", tc.script}}}, jobInputs{Output: func(b []byte) {
					if strings.Contains(string(b), "cancel-ready") {
						select {
						case started <- struct{}{}:
						default:
						}
					}
				}})
			}()
			defer func() { cancel(); <-done }()
			select {
			case <-started:
			case <-done:
				t.Fatalf("job exited before cancellation: %+v %v", res, err)
			case <-time.After(15 * time.Second):
				t.Fatal("job did not start")
			}
			cancel()
			select {
			case <-done:
				if err != nil || res.Attempt.State != "cancelled" {
					t.Fatalf("cancel result: %+v %v", res, err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("cancelled result missed the transport deadline")
			}
		})
	}
}
