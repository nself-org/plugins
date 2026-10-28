package main

// Purpose: the `--coordinator` sign source and the two release routes' wiring (P7-CACHE-17): the flag
// is registered once through publish.RegisterSignSource, a missing or unusable --out is a usage error
// before any credential is read, a plain-http coordinator is refused, and the routes answer "not
// found" until a serve has started and are cleared when it stops.
// Constraints: no network, no process; the signing itself is covered by the internal/release/remote
// integration tests (TestTeamSignRelease and friends).

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/coordinator/clientapi"
	"github.com/nself-org/plugins/free/ci/internal/release/publish"
	"github.com/nself-org/plugins/free/ci/internal/release/remote"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestSignCoordinatorSourceIsRegistered(t *testing.T) {
	found := false
	for _, s := range publish.SignSources() {
		found = found || s.Name == "coordinator"
	}
	if !found {
		t.Fatal("the coordinator sign source is not registered")
	}
}

// A companion flag of a source (--out of --coordinator) set alone must be refused: the local signer
// ignores it, so the command would exit 0 having written nothing to the directory (D-1357).
func TestSignCompanionFlagWithoutItsSourceIsRefused(t *testing.T) {
	t.Setenv("NSELF_CI_HOME", t.TempDir())
	t.Setenv("NSELF_V15", "1")
	for _, value := range []string{filepath.Join(t.TempDir(), "out"), ""} {
		code, stdout, errs := capture(t, func() int { return artifactsSign([]string{"run-1", "--out", value}) })
		if code != 1 || !strings.Contains(errs, "--out needs --coordinator") {
			t.Fatalf("lone --out %q: code %d stdout %q stderr %q", value, code, stdout, errs)
		}
		if value != "" {
			if _, err := filepath.EvalSymlinks(value); err == nil {
				t.Fatal("a refused sign created the output directory")
			}
		}
	}
}

func TestSignCoordinatorUsageAndRefusals(t *testing.T) {
	t.Setenv("NSELF_CI_HOME", t.TempDir())
	t.Setenv("NSELF_CI_COORDINATOR", "")
	t.Setenv("NSELF_V15", "1")
	fresh := filepath.Join(t.TempDir(), "out")
	for name, c := range map[string]struct {
		args []string
		want int
	}{
		"no --out":      {[]string{"run-1", "--coordinator", "https://127.0.0.1:1"}, 1},
		"plain http":    {[]string{"run-1", "--coordinator", "http://127.0.0.1:1", "--out", fresh}, 2},
		"no credential": {[]string{"run-1", "--coordinator", "https://127.0.0.1:1", "--out", fresh}, 2},
		"bad run id":    {[]string{"a b", "--coordinator", "https://127.0.0.1:1", "--out", fresh}, 2},
	} {
		if got := artifactsSign(c.args); got != c.want {
			t.Errorf("%s: exit %d, want %d", name, got, c.want)
		}
	}
	if exists := func() bool { _, err := filepath.EvalSymlinks(fresh); return err == nil }(); exists {
		t.Fatal("a refused sign created the output directory")
	}
}

func TestReleaseRoutesAnswerNotFoundUntilTheServeStarts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("NSELF_CI_HOME", home)
	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	svc := roles.New(st, &audit.Log{St: st})
	svc.Self = "alice"
	api := clientapi.New(clientapi.Config{Roles: svc, Local: true, Operator: "cli:alice", Backend: &clientapi.StoreBackend{Store: st, Home: home}})
	srv := httptest.NewServer(api.Handler())
	t.Cleanup(srv.Close)
	get := func(path string) (int, clientapi.Body) {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		raw, _ := io.ReadAll(resp.Body)
		var b clientapi.Body
		_ = json.Unmarshal(raw, &b)
		return resp.StatusCode, b
	}
	for _, path := range []string{remote.InputsPath("run-1"), remote.BlobPath("run-1", strings.Repeat("a", 64))} {
		if status, b := get(path); status != http.StatusNotFound || b.Code != "E713" {
			t.Fatalf("before the serve starts %s: %d %+v", path, status, b)
		}
	}
	co, err := coordinator.New(coordinator.Config{Store: st, PolicyDigest: "t"})
	if err != nil {
		t.Fatal(err)
	}
	stop, err := startReleaseRoutes(context.Background(), "on", co)
	if err != nil || releaseRouteDeps.Load() == nil {
		t.Fatalf("start: %v (deps %v)", err, releaseRouteDeps.Load())
	}
	if status, b := get(remote.InputsPath("run-1")); status != http.StatusNotFound || b.Code != "E713" {
		t.Fatalf("an unknown run with the serve started: %d %+v", status, b)
	}
	// the shipped wiring reads the coordinator's own store and the blob store under its CI home
	d := releaseRouteDeps.Load()
	if d.Store != any(st) || d.Runs == nil {
		t.Fatalf("deps %+v", d)
	}
	dirs, err := cas.New(records.StoreDir(serveHome()))
	if err != nil {
		t.Fatal(err)
	}
	digest, _, err := dirs.Put(context.Background(), cas.ArtifactRealm("p"), strings.NewReader("committed bytes"))
	if err != nil {
		t.Fatal(err)
	}
	rc, err := d.Blobs.Open(cas.ArtifactRealm("p"), digest)
	if err != nil {
		t.Fatalf("the route's blob store is not the coordinator's: %v", err)
	}
	_ = rc.Close()
	_ = stop(context.Background())
	if releaseRouteDeps.Load() != nil {
		t.Fatal("stopping the serve left the dependencies in place")
	}
	if off, err := startReleaseRoutes(context.Background(), "off", co); err != nil || releaseRouteDeps.Load() != nil || off == nil {
		t.Fatalf("off: %v", err)
	}
}
