package run

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/secrets"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

func operatorNode(id, kind string, eligible bool, ownership string) model.Capability {
	var n model.Capability
	n.Identity.ID, n.Identity.Transport = id, kind
	n.Identity.Ownership.Value = &ownership
	n.SecretsEligible = eligible
	return n
}

// D-1101: a secret job runs on a node when the coordinator is PERSONAL, and stays here under TEAM.
func TestLocalOnlySecretsFollowCoordinatorMode(t *testing.T) {
	rs := &runState{c: &Coordinator{}, hooks: Hooks{}}
	job := model.Job{Kind: "test", Run: []string{"true"}, Secrets: []string{"project/K"}}
	for mode, want := range map[string]bool{"": false, "personal": false, "team": true} {
		t.Setenv("NSELF_CI_COORDINATOR_MODE", mode)
		if got := rs.localOnly(job); got != want {
			t.Errorf("mode %q: localOnly = %v, want %v", mode, got, want)
		}
	}
}

// D-1101: the lease names the refs (never a value) for an operator-owned eligible node, and a node
// that may not take values is refused before the driver runs, never run without its secrets.
func TestDeliverCarriesSecretRefsOnlyToEligibleNode(t *testing.T) {
	probe := &probeDriver{kind: fmt.Sprintf("noderefs%d", time.Now().UnixNano())}
	if err := transport.Register(probe); err != nil {
		t.Fatal(err)
	}
	job := model.Job{Kind: "test", Run: []string{"true"}, Secrets: []string{"project/FOO"}}
	a := newTransportAdapter(nil, nil, "", nodeScope{})
	if err := a.Deliver(context.Background(), deliveryFor(operatorNode("n1", probe.kind, true, "operator"), job)); err != nil {
		t.Fatal(err)
	}
	probe.mu.Lock()
	got := probe.lease
	probe.mu.Unlock()
	if len(got.Secrets) != 1 || got.Secrets[0].Ref != "project/FOO" || got.Secrets[0].Class != "project" {
		t.Fatalf("lease secrets = %+v, want the project/FOO ref", got.Secrets)
	}
	probe.lease = protocol.Lease{}
	for name, n := range map[string]model.Capability{
		"foreign":      operatorNode("n2", probe.kind, false, "third-party"),
		"not eligible": operatorNode("n3", probe.kind, false, "operator"),
	} {
		if err := a.Deliver(context.Background(), deliveryFor(n, job)); err == nil {
			t.Errorf("%s node: a secret job was delivered", name)
		}
	}
	t.Setenv("NSELF_CI_COORDINATOR_MODE", "team")
	if err := a.Deliver(context.Background(), deliveryFor(operatorNode("n4", probe.kind, true, "operator"), job)); err == nil {
		t.Error("a TEAM coordinator delivered a secret job to a node")
	}
	probe.mu.Lock()
	defer probe.mu.Unlock()
	if probe.lease.LeaseID != "" {
		t.Errorf("the driver ran for a refused delivery: %+v", probe.lease)
	}
}

// D-1101: the per-lease source resolves the lease's refs here, hands the values to the driver and
// registers them with the redactor first; a node that may not take values resolves nothing.
func TestNodeSecretSourceResolvesLeaseRefs(t *testing.T) {
	cli := &fakeCli{}
	ConfigureSecrets(SecretsRuntime{Mode: secrets.ModePersonalLocal, Runner: cli})
	t.Cleanup(func() { ConfigureSecrets(SecretsRuntime{Mode: secrets.ModePersonalLocal}) })
	job := model.Job{Kind: "test", Run: []string{"true"}, Secrets: []string{"project/FOO"}}
	lease := protocol.Lease{LeaseID: "l1", AttemptID: "a1", Job: job, Secrets: []protocol.SecretRef{{Ref: "project/FOO", Class: "project"}}}
	scope := nodeScope{home: t.TempDir(), root: t.TempDir(), coordinator: "c1", node: operatorNode("n2", "ssh", false, "operator")}

	if vals, err := NodeSecretSource(withNodeScope(context.Background(), scope), lease); err == nil || vals != nil || cli.calls != 0 {
		t.Fatalf("a node that is not secrets-eligible got values %v (err %v, cli calls %d)", vals, err, cli.calls)
	}
	scope.node = operatorNode("n1", "ssh", true, "operator")
	vals, err := NodeSecretSource(withNodeScope(context.Background(), scope), lease)
	if err != nil {
		t.Fatal(err)
	}
	if len(vals) != 1 || vals[0].Ref != "project/FOO" || vals[0].Value != hookValue {
		t.Fatalf("values = %+v, want project/FOO", vals)
	}
	if got := string(Redactions.NewFilter().Redact([]byte("x " + hookValue + " y"))); strings.Contains(got, hookValue) {
		t.Fatalf("the value was not registered with the redactor: %q", got)
	}
}
