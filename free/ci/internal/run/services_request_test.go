package run

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/services"
)

// Each attempt has its own pool key; a request without an attempt id is refused rather than keyed by run and names.
func TestServiceKeyIsTheAttempt(t *testing.T) {
	cases := []struct {
		name    string
		a, b    ServicesRequest
		wantSet bool // the two requests get different keys
		wantErr bool
	}{
		{"two jobs of one run, same services", ServicesRequest{RunID: "r", AttemptID: "a1", Names: []string{"db"}}, ServicesRequest{RunID: "r", AttemptID: "a2", Names: []string{"db"}}, true, false},
		{"same attempt, names in another order", ServicesRequest{AttemptID: "a1", Names: []string{"db", "cache"}}, ServicesRequest{AttemptID: "a1", Names: []string{"cache", "db"}}, false, false},
		{"no attempt id", ServicesRequest{RunID: "r", Names: []string{"db"}}, ServicesRequest{RunID: "r", Names: []string{"db"}}, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ka, ea := serviceKey(tc.a)
			kb, eb := serviceKey(tc.b)
			if (ea != nil) != tc.wantErr || (eb != nil) != tc.wantErr {
				t.Fatalf("errors %v %v, want error %v", ea, eb, tc.wantErr)
			}
			if !tc.wantErr && (ka != kb) != tc.wantSet {
				t.Fatalf("keys %q %q, want different = %v", ka, kb, tc.wantSet)
			}
		})
	}
}

// The specs come from the repository root's config, whatever the working directory is.
func TestServiceSpecsAtReadsTheRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".nself"), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := "version: 1\nservices:\n  db:\n    component: postgres\n    version: 16-alpine\n"
	if err := os.WriteFile(filepath.Join(root, ".nself", "ci.yaml"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir()) // another tree entirely
	got := ServiceSpecsAt(root, []string{"db", "redis"})
	if len(got) != 2 || got[0] != (services.Spec{Name: "db", Component: "postgres", Version: "16-alpine"}) || got[1] != (services.Spec{Name: "redis"}) {
		t.Fatalf("specs %+v", got)
	}
	if cwd := ServiceSpecs([]string{"db"}); len(cwd) != 1 || cwd[0].Component != "" {
		t.Fatalf("the working directory has no config, so db is a plain component name: %+v", cwd)
	}
}

type recordingServices struct{ got []ServicesRequest }

func (r *recordingServices) Up(_ context.Context, q ServicesRequest) (ServicesSetup, error) {
	r.got = append(r.got, q)
	return ServicesSetup{}, nil
}
func (r *recordingServices) Down(context.Context, ServicesRequest) error { return nil }

// prepare hands the hook the attempt, the resolved specs, the isolation level and the network scope.
func TestPrepareFillsTheServicesRequest(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".nself"), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := "version: 1\nservices:\n  db:\n    component: postgres\n"
	if err := os.WriteFile(filepath.Join(root, ".nself", "ci.yaml"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	rec := &recordingServices{}
	rs := &runState{c: &Coordinator{Opts: Options{Root: root, Network: "none"}}, hooks: Hooks{Services: rec}}
	info := AttemptInfo{RunID: "run-1", AttemptID: "att-1", Job: model.Job{Isolation: "process", NeedsServices: []string{"db"}}}
	if _, err := rs.prepare(context.Background(), &info); err != nil {
		t.Fatal(err)
	}
	if len(rec.got) != 1 {
		t.Fatalf("hook calls: %d", len(rec.got))
	}
	q := rec.got[0]
	if q.RunID != "run-1" || q.AttemptID != "att-1" || q.Isolation != "process" || q.Network != "none" {
		t.Errorf("request %+v", q)
	}
	if len(q.Specs) != 1 || q.Specs[0] != (services.Spec{Name: "db", Component: "postgres"}) {
		t.Errorf("specs %+v", q.Specs)
	}
}
