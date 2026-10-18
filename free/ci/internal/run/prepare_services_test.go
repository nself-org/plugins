package run

// Purpose: the services hook request names the attempt, the effective
// isolation, the network scope and the resolved specs (D-0975), and two
// attempts of one run asking for the same services never share a key.

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/services"
)

type capSvc struct{ reqs []ServicesRequest }

func (c *capSvc) Up(_ context.Context, r ServicesRequest) (ServicesSetup, error) {
	c.reqs = append(c.reqs, r)
	return ServicesSetup{Env: map[string]string{"NSELF_CI_SERVICE_DB_HOST": "h"}}, nil
}
func (c *capSvc) Down(context.Context, ServicesRequest) error { return nil }

func rootWithDeclared(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".nself"), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := "version: 1\nservices:\n  db:\n    component: postgres\n    version: 16-alpine\n"
	if err := os.WriteFile(filepath.Join(root, ".nself", "ci.yaml"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestPrepareFillsTheServicesRequest(t *testing.T) {
	root := rootWithDeclared(t)
	t.Chdir(t.TempDir()) // the specs come from the repository root, not the cwd
	cs := &capSvc{}
	rs := &runState{c: &Coordinator{Opts: Options{Root: root, Network: "restricted", Services: []string{"redis"}}}, hooks: Hooks{Services: cs}}
	info := &AttemptInfo{RunID: "r1", AttemptID: "a1", Job: model.Job{NeedsServices: []string{"db"}, Isolation: "process"}}
	if _, err := rs.prepare(context.Background(), info); err != nil {
		t.Fatal(err)
	}
	if len(cs.reqs) != 1 {
		t.Fatalf("want one Up, got %d", len(cs.reqs))
	}
	got := cs.reqs[0]
	if got.RunID != "r1" || got.AttemptID != "a1" {
		t.Fatalf("run and attempt id: %+v", got)
	}
	if got.Network != model.NetworkScope("restricted") || got.Isolation != "process" {
		t.Fatalf("network and isolation: %+v", got)
	}
	want := []services.Spec{{Name: "db", Component: "postgres", Version: "16-alpine"}, {Name: "redis"}}
	if !reflect.DeepEqual(got.Specs, want) {
		t.Fatalf("specs = %+v, want %+v", got.Specs, want)
	}
}

func TestPrepareRequestIsolationFollowsContainerFlag(t *testing.T) {
	cs := &capSvc{}
	rs := &runState{c: &Coordinator{Opts: Options{Root: t.TempDir(), Container: true, Network: "none"}}, hooks: Hooks{Services: cs}}
	info := &AttemptInfo{RunID: "r1", AttemptID: "a1", Job: model.Job{NeedsServices: []string{"db"}, Container: &model.ContainerConfig{Image: "img"}}}
	if _, err := rs.prepare(context.Background(), info); err != nil {
		t.Fatal(err)
	}
	if got := cs.reqs[0].Isolation; got != "container" {
		t.Fatalf("isolation = %q, want container", got)
	}
}

func TestServiceKeyIsTheAttempt(t *testing.T) {
	a := ServicesRequest{RunID: "r", AttemptID: "a1", Names: []string{"db", "redis"}}
	b := ServicesRequest{RunID: "r", AttemptID: "a2", Names: []string{"redis", "db"}}
	ka, err := serviceKey(a)
	if err != nil {
		t.Fatal(err)
	}
	kb, _ := serviceKey(b)
	if ka == kb {
		t.Fatal("two attempts of one run with the same names must not share a key")
	}
	if _, err := serviceKey(ServicesRequest{RunID: "r", Names: []string{"db"}}); err == nil {
		t.Fatal("a request without an attempt id must be refused")
	}
}
