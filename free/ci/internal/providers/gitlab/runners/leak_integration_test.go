//go:build integration

package runners_test

// Purpose: the no-leak acceptance test (3): the glrt- token never appears where a node, a log or the
// coordinator can see it.
// Inputs:  the sim fixtures and the container leg.
// Outputs: pass or fail.
// Constraints: Docker is required; every scan has a positive control and counts the entries it read.

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/runners"
	"github.com/nself-org/plugins/free/ci/internal/providers/runnerjob"
)

// TestNoTokenLeak proves acceptance 3: the minted token never appears in the node's ps output, `docker
// inspect` of the job container (no CI_SERVER_TOKEN variable either), the store, the agent state directory,
// the log or the result, whether the run ends well or the registration is torn down afterwards. The stub
// runner (a pinned busybox) records that the token IS visible in its own process environment: the stated
// residual, since the job shares the runner's container and user and the runner is paused when its job runs.
// The mint and delete requests carry only the runner-management token (TestRunnerTokenOnly). Each scan has a
// positive control (the planted value is found) and counts the entries it read.
func TestNoTokenLeak(t *testing.T) {
	needDocker(t, stubImage)
	st, storeDir := useSimAt(t)
	f := newFakeGL(t)
	reg := f.registrar(clock)
	runners.UseRegistrar(func(context.Context) (*runners.Registrar, error) { return reg, nil })
	t.Cleanup(func() { runners.UseRegistrar(nil) })
	lease, _ := admitAndDeliver(t, st, "leak-attempt", "51", "nself")
	got := deliverLease(t, goodNode(), lease)
	if got.RunnerCredential == nil {
		t.Fatal("no credential minted")
	}
	cred := got.RunnerCredential.Value
	for _, sandboxed := range []bool{false, true} {
		id := map[bool]string{false: "host07-leak-c", true: "host07-leak-g"}[sandboxed]
		run, ran := runLeg(t, legOptions{ID: id, Image: stubImage, Cred: cred, Command: []string{"sh", "-c", stubScript}, Sandboxed: sandboxed, Limit: 90 * time.Second})
		if !ran {
			continue
		}
		if run.Res.Err != nil || run.Res.ExitCode != 0 {
			t.Fatalf("level %s: %+v\n%s", run.Level, run.Res, run.Out)
		}
		if !strings.Contains(run.Out, "visible-in-env=1") || !strings.Contains(run.Out, "len="+itoa(int64(len(cred)))) || !strings.Contains(run.Out, "echoed [redacted]") {
			t.Fatalf("level %s: the runner must see the token in its own env, and the log must mask it:\n%s", run.Level, run.Out)
		}
		if run.Inspect == "" || run.Ps == "" {
			t.Fatalf("level %s: the sampler saw no container (inspect %d bytes, ps %d bytes)", run.Level, len(run.Inspect), len(run.Ps))
		}
		for name, text := range map[string]string{"ps": run.Ps, "docker inspect": run.Inspect, "log": run.Out, "result": strings.Join([]string{run.Res.Reason, run.Res.Signal, run.Res.Attempt.ID}, " ")} {
			if strings.Contains(text, cred) {
				t.Errorf("level %s: the token is in the %s", run.Level, name)
			}
		}
		if envs := containerEnvs(t, run.Inspect); len(envs) == 0 || strings.Contains(strings.Join(envs, "\n"), "CI_SERVER_TOKEN") {
			t.Errorf("level %s: the container environment must be readable and carry no CI_SERVER_TOKEN (the credential is not a container variable): %v", run.Level, envs)
		}
		t.Logf("level %s: agent state dir while the job ran: %d entries read, %d hits", run.Level, run.HomeRead, len(run.HomeHits))
		if run.HomeRead == 0 || len(run.HomeHits) != 0 {
			t.Errorf("level %s: agent state dir: %d entries read, token in %v", run.Level, run.HomeRead, run.HomeHits)
		}
	}
	// the teardown and the ledger: remove the registration, then scan the store and what the ledger kept
	if err := runnerjob.Teardown(context.Background(), runnerjob.CurrentLedger(), "leak-attempt"); err != nil {
		t.Fatal(err)
	}
	if f.count() != 0 {
		t.Fatalf("the runner must be gone: %v", f.descriptions())
	}
	read, hits := scanTree(storeDir, cred)
	t.Logf("store dir: %d entries read, %d hits", read, len(hits))
	if read == 0 || len(hits) != 0 {
		t.Errorf("store: %d entries read, token in %v", read, hits)
	}
	// positive control: the scan finds a planted value, so a clean result above means something
	planted := filepath.Join(t.TempDir(), "planted")
	if err := os.WriteFile(planted, []byte("x "+cred+" y"), 0o600); err != nil {
		t.Fatal(err)
	}
	if n, found := scanTree(filepath.Dir(planted), cred); n < 2 || len(found) != 1 {
		t.Fatalf("positive control failed: read %d found %v", n, found)
	}
}

// containerEnvs lists the environment variables `docker inspect` showed (the sampler keeps one JSON array per sample).
func containerEnvs(t *testing.T, inspect string) []string {
	t.Helper()
	var all []string
	d := json.NewDecoder(strings.NewReader(inspect))
	for {
		var rows []struct {
			Config struct{ Env []string }
		}
		if err := d.Decode(&rows); err != nil {
			if err != io.EOF {
				t.Logf("inspect sample not JSON: %v", err)
			}
			return all
		}
		for _, r := range rows {
			all = append(all, r.Config.Env...)
		}
	}
}
