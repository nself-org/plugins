package main

// Purpose: D-1534 on the leased node: a TEAM coordinator's lease carries references only and this node
//          resolves them itself, under its own age identity, when its eligibility, its grant and the lease's
//          trust class allow; anything else is refused before a value is read.
// Inputs:  a fake nself cli (contract:cli.secret-resolve v1) that counts its calls and can refuse a node
//          its identity was not granted. Constraints: no process is started; values are synthetic.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/secrets"
)

const teamValue = "team-node-fake-value-4c19e2"

// teamCli is the node's nself cli. ungranted answers every resolve with a cli error, as a store file
// this node's age identity cannot decrypt does.
type teamCli struct {
	calls, resolves int
	ungranted       bool
}

func (c *teamCli) Run(_ context.Context, call secrets.Call) (secrets.Output, error) {
	c.calls++
	argv := call.Argv
	if argv[len(argv)-1] == "--help" {
		return secrets.Output{Bytes: []byte("usage")}, nil
	}
	c.resolves++
	if c.ungranted {
		b, _ := json.Marshal(map[string]any{"schema_version": "1", "command": "config secrets resolve", "error": map[string]string{"code": "E543", "message": "fixed text"}})
		return secrets.Output{Bytes: b, ExitCode: 1}, nil
	}
	var attempt string
	var vals []map[string]any
	for i, a := range argv {
		if a == "--for-attempt" {
			attempt = argv[i+1]
		}
		if a == "--json" {
			for _, ref := range argv[i+1:] {
				r, _ := secrets.Parse(ref)
				vals = append(vals, map[string]any{"ref": ref, "env_name": r.Name, "value": teamValue, "source": "age:ci.age", "expires_at": nil})
			}
		}
	}
	b, _ := json.Marshal(map[string]any{"schema_version": "1", "command": "config secrets resolve",
		"data": map[string]any{"schema": "cli.secret-resolve/v1", "attempt_id": attempt, "values": vals}})
	return secrets.Output{Bytes: b}, nil
}

func withTeamCli(t *testing.T, c *teamCli) {
	t.Helper()
	old := nodeSecretsRunner
	nodeSecretsRunner = func(string, string, string) secrets.Runner { return c }
	t.Cleanup(func() { nodeSecretsRunner = old })
}

func TestTeamLeaseResolvesOnTheEligibleNode(t *testing.T) {
	cli := &teamCli{}
	withTeamCli(t, cli)
	f := pullFlags{project: t.TempDir(), secretsOK: true, coordinator: "team-c1"}
	resolve := nodeResolver(t.TempDir(), f, "n-1", "/var/lib/node/age.key")
	got, err := resolve(context.Background(), "a1", "internal", []string{"project/API_TOKEN"})
	if err != nil || got["API_TOKEN"] != teamValue {
		t.Fatalf("got %v err %v; want the node-resolved value", got, err)
	}
	if cli.resolves != 1 {
		t.Fatalf("resolve calls = %d, want 1", cli.resolves)
	}
}

func TestTeamLeaseRefusedBeforeAnyValueIsRead(t *testing.T) {
	for _, tc := range []struct {
		name     string
		eligible bool
		class    model.TrustClass
		ref      string
		reason   string
	}{
		{"untrusted run", true, "untrusted", "project/API_TOKEN", "secrets.untrusted"},
		{"node not secrets-eligible", false, "internal", "project/API_TOKEN", secrets.ReasonNodeNotEligible},
		{"release class on a shared node", true, "internal", "release/SIGNING_KEY", "trust.release_needs_dedicated"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cli := &teamCli{}
			withTeamCli(t, cli)
			f := pullFlags{project: t.TempDir(), secretsOK: tc.eligible, coordinator: "team-c1"}
			got, err := nodeResolver(t.TempDir(), f, "n-1", "/var/lib/node/age.key")(context.Background(), "a1", tc.class, []string{tc.ref})
			var se *secrets.Error
			if got != nil || !asSecretsError(err, &se) || se.Code != "E677" || !strings.Contains(strings.Join(se.Reasons, ","), tc.reason) {
				t.Fatalf("got %v err %v; want E677 %s", got, err, tc.reason)
			}
			if cli.calls != 0 {
				t.Fatalf("the cli was called %d times before the refusal", cli.calls)
			}
		})
	}
}

func TestTeamLeaseOnAnUngrantedNodeGetsNoValue(t *testing.T) {
	cli := &teamCli{ungranted: true}
	withTeamCli(t, cli)
	f := pullFlags{project: t.TempDir(), secretsOK: true, coordinator: "team-c1"}
	got, err := nodeResolver(t.TempDir(), f, "n-1", "/var/lib/node/age.key")(context.Background(), "a1", "internal", []string{"project/API_TOKEN"})
	if got != nil || secrets.CodeOf(err) != "E676" || strings.Contains(err.Error(), teamValue) {
		t.Fatalf("got %v err %v; want E676 and no value", got, err)
	}
}

func asSecretsError(err error, out **secrets.Error) bool {
	e, ok := err.(*secrets.Error)
	*out = e
	return ok
}
