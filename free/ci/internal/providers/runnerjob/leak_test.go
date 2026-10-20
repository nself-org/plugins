//go:build integration

package runnerjob_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers/runnerjob"
)

// scanTree counts the entries (files and directories) under root and lists the files that contain needle.
func scanTree(root, needle string) (read int, hits []string) {
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		read++
		if d.IsDir() || d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if b, rerr := os.ReadFile(p); rerr == nil {
			if strings.Contains(string(b), needle) {
				hits = append(hits, p)
			}
		}
		return nil
	})
	return read, hits
}

const stubScript = `echo "visible-in-env=$(env | grep -c '^ACTIONS_RUNNER_INPUT_JITCONFIG=')"; ` +
	`echo "len=${#ACTIONS_RUNNER_INPUT_JITCONFIG}"; echo "echoed ${ACTIONS_RUNNER_INPUT_JITCONFIG}"; sleep 3`

// TestNoCredentialLeak proves acceptance line 4 (review F6): the minted value never appears in the node's
// ps output, `docker inspect` of the job container, the store, the agent state directory, the logs or the
// evidence, whether the run ends well or the registration is torn down afterwards. The stub runner (a
// pinned busybox) records that the value IS visible in its own process environment: that is the stated
// residual, since the job shares the runner's container and user; the registration admits exactly this one
// job. Each scan has a positive control (the planted value is found) and counts the files it read.
func TestNoCredentialLeak(t *testing.T) {
	needDocker(t, stubImage)
	st := use(t)
	l, err := deliver(goodNode(), "host19-leak", runnerJob(t, "51"))
	if err != nil || l.RunnerCredential == nil {
		t.Fatalf("deliver: %v", err)
	}
	cred := l.RunnerCredential.Value
	for _, sandboxed := range []bool{false, true} {
		id := map[bool]string{false: "host19-leak-c", true: "host19-leak-g"}[sandboxed]
		run, ran := runLeg(t, id, stubImage, "github-jitconfig", cred, []string{"sh", "-c", stubScript}, sandboxed, 90*time.Second)
		if !ran {
			continue
		}
		t.Logf("level %s: stub says %q", run.Level, strings.Fields(run.Out))
		if run.Res.Err != nil || run.Res.ExitCode != 0 {
			t.Fatalf("level %s: %+v\n%s", run.Level, run.Res, run.Out)
		}
		if !strings.Contains(run.Out, "visible-in-env=1") || !strings.Contains(run.Out, "len="+itoa(len(cred))) || !strings.Contains(run.Out, "echoed [redacted]") {
			t.Fatalf("level %s: the runner must see the value in its own env, and the log must mask it:\n%s", run.Level, run.Out)
		}
		if run.Inspect == "" || run.Ps == "" {
			t.Fatalf("level %s: the sampler saw no container (inspect %d bytes, ps %d bytes)", run.Level, len(run.Inspect), len(run.Ps))
		}
		for name, text := range map[string]string{"ps": run.Ps, "docker inspect": run.Inspect, "log": run.Out, "result": strings.Join([]string{run.Res.Reason, run.Res.Signal, run.Res.Attempt.ID}, " ")} {
			if strings.Contains(text, cred) {
				t.Errorf("level %s: the credential is in the %s", run.Level, name)
			}
		}
		t.Logf("level %s: agent state dir while the job ran: %d entries read, %d hits", run.Level, run.HomeRead, len(run.HomeHits))
		if run.HomeRead == 0 || len(run.HomeHits) != 0 {
			t.Errorf("level %s: agent state dir: %d entries read, credential in %v", run.Level, run.HomeRead, run.HomeHits)
		}
	}
	// the teardown and the ledger: remove the registration, then scan the store and what the ledger kept
	if err = runnerjob.Teardown(context.Background(), runnerjob.CurrentLedger(), "host19-leak"); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(storePath(st))
	read, hits := scanTree(dir, cred)
	t.Logf("store dir: %d entries read, %d hits", read, len(hits))
	if read == 0 || len(hits) != 0 {
		t.Errorf("store: %d entries read, credential in %v", read, hits)
	}
	// positive control: the scan finds a planted value, so a clean result above means something
	planted := filepath.Join(t.TempDir(), "planted")
	if err = os.WriteFile(planted, []byte("x "+cred+" y"), 0o600); err != nil {
		t.Fatal(err)
	}
	if n, found := scanTree(filepath.Dir(planted), cred); n < 2 || len(found) != 1 {
		t.Fatalf("positive control failed: read %d found %v", n, found)
	}
}
