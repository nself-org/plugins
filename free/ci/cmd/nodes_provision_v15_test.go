package main

// Purpose: a provisioned runner is recorded in the real registry and shows in `nodes list --json` with
// trust.accepts null, never eligible (P7-NODE-06). Inputs: a temp NSELF_CI_HOME. Constraints: no ssh.

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
	"github.com/nself-org/plugins/free/ci/internal/nodes/provision"
)

func TestProvisionedRunnerShowsInNodesListWithNullAccepts(t *testing.T) {
	t.Setenv("NSELF_CI_HOME", t.TempDir())
	t.Setenv("NSELF_CI_REPO", t.TempDir())
	ctx := context.Background()
	id, err := recordRunnerNode(ctx, provision.RecordRequest{Host: "ci@runner-7", Labels: []string{"gpu"}})
	if err != nil || id != "gh-runner-runner-7" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	if again, err := recordRunnerNode(ctx, provision.RecordRequest{Host: "ci@runner-7"}); err != nil || again != id {
		t.Fatalf("a second provision of the same host: %q %v", again, err)
	}
	var out, errb bytes.Buffer
	env := lifecycle.ProductionEnv(currentProject)
	env.Out, env.Err = &out, &errb
	if code := lifecycle.Run("list", []string{"--json"}, env); code != 0 {
		t.Fatalf("list exit %d: %s", code, errb.String())
	}
	var doc struct {
		Data struct {
			Nodes []map[string]any `json:"nodes"`
		} `json:"data"`
	}
	if err = json.Unmarshal(out.Bytes(), &doc); err != nil || len(doc.Data.Nodes) != 1 {
		t.Fatalf("list output: %v\n%s", err, out.String())
	}
	n := doc.Data.Nodes[0]
	trust, _ := n["trust"].(map[string]any)
	if n["id"] != id || n["transport"] != "provider" || n["eligible"] != false || trust == nil {
		t.Fatalf("node = %v", n)
	}
	if v, present := trust["accepts"]; !present || v != nil {
		t.Fatalf("trust.accepts = %v (present=%v), want null", v, present)
	}
}
