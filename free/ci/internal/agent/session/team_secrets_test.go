package session

// Purpose: D-1534 end to end on a fake leased node: a TEAM coordinator's lease carries references only,
//          the node's own resolver (transport/agent.WithNodeSecrets) turns them into values inside the job's
//          setup, the job sees the value, and no frame the node sends back (logs, result, evidence) holds it.
// Inputs:  the session rig on a fake carrier wrapped by WithNodeSecrets; a resolver that stands in for the
//          node's `nself config secrets resolve`. Constraints: values are synthetic; bounded waits.

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/transport/agent"
)

const nodeResolvedValue = "node-resolved-fake-value-9d2a71"

// nodeSecretsCarrier is the pull carrier with node-side resolution, as `ci agent pull` builds it.
type nodeSecretsCarrier struct {
	agent.Frames
	fc *fakeCarrier
}

func (c nodeSecretsCarrier) fake() *fakeCarrier { return c.fc }

func TestTeamLeaseSecretResolvedOnTheNodeAndMasked(t *testing.T) {
	var mu sync.Mutex
	var gotClass model.TrustClass
	var gotRefs []string
	resolve := func(_ context.Context, _ string, class model.TrustClass, refs []string) (map[string]string, error) {
		mu.Lock()
		defer mu.Unlock()
		gotClass, gotRefs = class, refs
		return map[string]string{"API_TOKEN": nodeResolvedValue}, nil
	}
	seen := make(chan map[string]string, 1)
	runner := RunnerFunc(func(ctx context.Context, j JobRun) (JobResult, error) {
		seen <- j.Setup.Secrets
		j.Output([]byte("token=" + j.Setup.Secrets["API_TOKEN"] + "\n"))
		return JobResult{Outcome: Outcome{State: "passed"}, Evidence: json.RawMessage(`{"note":"done"}`)}, nil
	})
	fc := newFakeCarrier()
	r := startRigWith(t, t.TempDir(), nodeSecretsCarrier{Frames: agent.WithNodeSecrets(fc, resolve, nil), fc: fc}, runner)
	r.handshake()
	l := testLease("l1", 1)
	l.Trust = "internal"
	l.Job.Secrets = []string{"project/API_TOKEN"}
	l.Secrets = []protocol.SecretRef{{Ref: "project/API_TOKEN", Class: "project"}}
	coordinatorLease, err := json.Marshal(l)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(coordinatorLease), nodeResolvedValue) {
		t.Fatal("the coordinator's lease holds a value")
	}
	r.lease(l)
	got := <-seen
	if got["API_TOKEN"] != nodeResolvedValue || got["project/API_TOKEN"] != nodeResolvedValue {
		t.Fatalf("the job did not see the node-resolved value: %v", len(got))
	}
	var logs []string
	for {
		m := <-fc.out
		raw, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), nodeResolvedValue) {
			t.Fatalf("a %s frame sent to the coordinator holds the value", m.Type)
		}
		if lg, ok := m.Body.(protocol.Log); ok {
			logs = append(logs, lg.Text)
		}
		if m.Type == "result" {
			break
		}
	}
	if strings.Join(logs, "|") != "token=[redacted]" {
		t.Fatalf("job output not masked: %q", logs)
	}
	mu.Lock()
	defer mu.Unlock()
	if gotClass != "internal" || len(gotRefs) != 1 || gotRefs[0] != "project/API_TOKEN" {
		t.Fatalf("resolver saw class %q refs %v", gotClass, gotRefs)
	}
}
