package main

// Purpose: tests of `ci nodes sync` (P7-AUTO-04): parsing, the dry-run summary, the inventory refusal
//          codes and the TEAM refusal, against a real store and a fake discovery runner.
// Inputs:  inline inventory documents; a temp store through lifecycle.Env.
// Outputs: pass or fail.
// Constraints: no network, no processes, no sleeps, V15 handler only.

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/nodes/inventory"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const syncDoc = "schema: inventory/v1\nentries:\n  - id: vps1\n    ssh: ci@vps1.example.test\n    rank: 2\n"

func syncEnv(t *testing.T) (lifecycle.Env, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return lifecycle.Env{Open: func(context.Context, string) (*lifecycle.Deps, func(), error) {
		return lifecycle.NewDeps(st, t.TempDir(), "", "cli:test"), func() {}, nil
	}}, st
}

func writeInventory(t *testing.T, doc string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "inv.yaml")
	if err := os.WriteFile(p, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func syncJSON(t *testing.T, out string) (cmd string, data inventory.Summary, errCode string) {
	t.Helper()
	var env struct {
		Command string            `json:"command"`
		Data    inventory.Summary `json:"data"`
		Error   struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("envelope %q: %v", out, err)
	}
	return env.Command, env.Data, env.Error.Code
}

func TestNodesSyncDryRun(t *testing.T) {
	env, st := syncEnv(t)
	var out, errOut bytes.Buffer
	if code := runNodesSync([]string{"--from", writeInventory(t, syncDoc), "--dry-run", "--json"}, env, false, nil, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s %s", code, out.String(), errOut.String())
	}
	cmd, sum, _ := syncJSON(t, out.String())
	if cmd != "ci nodes sync" || !sum.DryRun || len(sum.Candidates) != 1 || sum.Candidates[0].Created {
		t.Fatalf("summary: %+v", sum)
	}
	if nodes, _ := st.NodeList(context.Background()); len(nodes) != 0 {
		t.Fatal("dry-run registered a node")
	}
	// The real run creates the candidate; the table form names it.
	out.Reset()
	if code := runNodesSync([]string{"--from=" + writeInventory(t, syncDoc)}, env, false, nil, &out, &errOut); code != 0 || !strings.Contains(out.String(), "candidate vps1 (created true)") {
		t.Fatalf("exit %d: %s", code, out.String())
	}
	if nodes, _ := st.NodeList(context.Background()); len(nodes) != 1 {
		t.Fatal("candidate not registered")
	}
}

func TestNodesSyncCmdAndRefusals(t *testing.T) {
	env, st := syncEnv(t)
	var gotArgv []string
	run := func(_ context.Context, argv []string) (string, int, error) { gotArgv = argv; return syncDoc, 0, nil }
	var out, errOut bytes.Buffer
	if code := runNodesSync([]string{"--json", "--cmd", "tool", "--json", "list"}, env, false, run, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, out.String())
	}
	if strings.Join(gotArgv, " ") != "tool --json list" {
		t.Fatalf("everything after --cmd is the command: %v", gotArgv)
	}
	bad := func(*testing.T) (string, int, error) { return "schema: inventory/v1\nbogus: 1\n", 0, nil }
	out.Reset()
	if code := runNodesSync([]string{"--json", "--cmd", "tool"}, env, false, func(context.Context, []string) (string, int, error) { return bad(t) }, &out, &errOut); code != 1 {
		t.Fatalf("unknown field exit %d", code)
	}
	if _, _, ec := syncJSON(t, out.String()); ec != inventory.CodeInventory {
		t.Fatalf("code = %q in %s", ec, out.String())
	}
	before, _ := st.NodeList(context.Background())
	if len(before) != 1 {
		t.Fatalf("a refused command applied something: %d nodes", len(before))
	}
	for name, args := range map[string][]string{"none": {}, "both": {"--from", "a", "--cmd", "b"}, "unknown flag": {"--bogus"}, "from without value": {"--from"}} {
		if code := runNodesSync(args, env, false, run, &out, &errOut); code != 1 {
			t.Errorf("%s: exit %d", name, code)
		}
	}
}

func TestNodesSyncRefusedOnCoordinator(t *testing.T) {
	env, st := syncEnv(t)
	var out, errOut bytes.Buffer
	if code := runNodesSync([]string{"--from", writeInventory(t, syncDoc), "--json"}, env, true, nil, &out, &errOut); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if _, _, ec := syncJSON(t, out.String()); ec != inventory.CodeInventory {
		t.Fatalf("code = %q", ec)
	}
	if nodes, _ := st.NodeList(context.Background()); len(nodes) != 0 {
		t.Fatal("a TEAM coordinator registered a node")
	}
}
