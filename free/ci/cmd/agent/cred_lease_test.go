//go:build !windows

package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// TestServeDeliversARunnerCredentialThroughTheWrapper proves D-1001: a lease with a
// runner_credential runs (it used to fail closed), the job sees the value only in the
// environment cred-wrapper sets, never in its argv, and the credential never reaches the log
// or the evidence unmasked.
func TestServeDeliversARunnerCredentialThroughTheWrapper(t *testing.T) {
	skipUnixOnly(t)
	useState(t)
	t.Setenv("PATH", filepath.Dir(mustLook(t, "sh"))+string(filepath.ListSeparator)+filepath.Dir(mustLook(t, "ps")))
	const secret = "JITSECRET-synthetic-0001"
	p := startServe(t)
	p.next("hello")
	p.send("welcome", protocol.Welcome{Version: 1, MinAgentVersion: "1.4.0", SessionID: "s", HeartbeatMS: 60000, AgentTTLMS: 120000}, 0, "")
	script := `[ "$ACTIONS_RUNNER_INPUT_JITCONFIG" = ` + secret + ` ] && echo credential-delivered; ps -o args= -p $$; echo "echoed $ACTIONS_RUNNER_INPUT_JITCONFIG"`
	lease := protocol.Lease{LeaseID: "l1", AttemptID: "attempt-l1", Epoch: 1, Token: "T", CompatMode: protocol.Compat15,
		Job:      model.Job{Kind: "static", Run: []string{"sh", "-c", script}, Env: map[string]string{"NSELF_AGENT_TEST_CHILD": "1"}},
		Checkout: protocol.Checkout{Revision: rev40}, Secrets: []protocol.SecretRef{}, ExpiresAt: "2030-01-01T00:00:00Z",
		RunnerCredential: &protocol.RunnerCredential{Kind: "github-jitconfig", Value: secret, ExpiresAt: "2030-01-01T00:00:00Z"}}
	p.send("lease", lease, 1, "l1")
	var logs []string
	var res *protocol.Result
	for res == nil {
		m, err := p.dec.Decode()
		if err != nil {
			t.Fatal(err)
		}
		switch b := m.Body.(type) {
		case *protocol.Log:
			logs = append(logs, b.Text)
		case *protocol.Result:
			res = b
		}
	}
	all := strings.Join(logs, "\n")
	if validEvidence(t, res.Evidence)["result"] != "pass" || !strings.Contains(all, "credential-delivered") {
		t.Fatalf("the credential was not delivered (evidence %s):\n%s", res.Evidence, all)
	}
	if strings.Contains(all, secret) || strings.Contains(string(res.Evidence), secret) || !strings.Contains(all, "echoed [redacted]") {
		t.Fatalf("credential leaked or not masked:\n%s\n%s", all, res.Evidence)
	}
}
