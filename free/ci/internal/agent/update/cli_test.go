package update

// Purpose: the command surface: argument errors and exit classes, and which nodes `--all` picks (ssh and reachable pull nodes only;
//          a pull node this process cannot reach is skipped and named, a revoked node is never touched).

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/agent/verify"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func registryDeps(t *testing.T) *lifecycle.Deps {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return lifecycle.NewDeps(st, t.TempDir(), "proj", "cli:test")
}

func addNode(t *testing.T, d *lifecycle.Deps, id, transport string) {
	t.Helper()
	b, err := os.ReadFile("../../model/testdata/capability/valid/laptop.valid.json")
	if err != nil {
		t.Fatal(err)
	}
	var c model.Capability
	if err = json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	c.Identity.ID, c.Identity.Name, c.Identity.Transport = id, id, transport
	if _, err = d.Registry.Register(context.Background(), c, "operator"); err != nil {
		t.Fatal(err)
	}
}

type fakeCtl struct{ AgentControl }

func TestUpgradableSkipsUnreachablePullAndRevokedNodes(t *testing.T) {
	d := registryDeps(t)
	addNode(t, d, "box-ssh", "ssh")
	addNode(t, d, "box-pull", "agent")
	addNode(t, d, "box-local", "local")
	addNode(t, d, "box-gone", "ssh")
	if _, err := d.Registry.SetState(context.Background(), "box-gone", "revoked", "test", "cli:test"); err != nil {
		t.Fatal(err)
	}
	RegisterAgentControl(nil)
	ids, skipped, err := upgradable(context.Background(), d)
	if err != nil || strings.Join(ids, ",") != "box-ssh" || strings.Join(skipped, ",") != "box-pull" {
		t.Fatalf("ids %v skipped %v err %v", ids, skipped, err)
	}
	RegisterAgentControl(fakeCtl{})
	t.Cleanup(func() { RegisterAgentControl(nil) })
	ids, skipped, err = upgradable(context.Background(), d)
	if err != nil || strings.Join(ids, ",") != "box-pull,box-ssh" || len(skipped) != 0 {
		t.Fatalf("ids %v skipped %v err %v", ids, skipped, err)
	}
}

func TestRunRejectsBadArguments(t *testing.T) {
	env := Env{DefaultVersion: "", Verifier: func() (*verify.Verifier, error) { return nil, nil }}
	for name, c := range map[string]struct {
		args []string
		exit int
		want string
	}{
		"no version pinned":         {[]string{"n1"}, 1, "E651"},
		"neither node nor --all":    {[]string{"--version", "1.5.0"}, 1, "E651"},
		"both a node and --all":     {[]string{"--version", "1.5.0", "--all", "n1"}, 1, "E651"},
		"two nodes":                 {[]string{"--version", "1.5.0", "n1", "n2"}, 1, "E651"},
		"version that is not plain": {[]string{"--version", "latest", "n1"}, 1, "E651"},
	} {
		t.Run(name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if code := Run(c.args, env, &out, &errOut); code != c.exit || !strings.Contains(errOut.String(), c.want) {
				t.Fatalf("code %d stderr %q", code, errOut.String())
			}
		})
	}
	var out, errOut bytes.Buffer
	if code := Run([]string{"--json", "--version", "1.5.0"}, env, &out, &errOut); code != 1 || !strings.Contains(out.String(), "E651") {
		t.Fatalf("--json error envelope: code %d out %q", code, out.String())
	}
}

func TestVersionFlagAcceptsALeadingV(t *testing.T) {
	var out, errOut bytes.Buffer
	env := Env{Verifier: func() (*verify.Verifier, error) { return nil, nil }}
	// the version is valid, so the run reaches the store (which this env cannot open): not the "no release is pinned" usage error
	env.Lifecycle.Open = func(context.Context, string) (*lifecycle.Deps, func(), error) { return nil, nil, os.ErrNotExist }
	code := Run([]string{"--version", "v1.5.0", "n1"}, env, &out, &errOut)
	if code != 2 || !strings.Contains(errOut.String(), "E607") {
		t.Fatalf("code %d stderr %q", code, errOut.String())
	}
}
