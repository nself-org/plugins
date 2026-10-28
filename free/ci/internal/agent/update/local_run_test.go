package update

// Purpose: ApplyLocal with the RunVersion seam (D-1385): the verified release is run before the swap and the installed file after
//          it, and a binary that does not start or reports another version is never left in place.
// Inputs:  the sim node home, signed stub releases and fake RunVersion funcs that answer per path.
// Outputs: pass or fail.
// Constraints: the real seam (internal/exec.RunAgentVersion) is tested in its own package and through cmd/agent.

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
)

// runLog is a RunVersion that records the paths it ran and answers from fail.
type runLog struct {
	mu    sync.Mutex
	paths []string
	fail  func(path string, n int) (string, error)
}

func (r *runLog) run(_ context.Context, bin string) (string, error) {
	r.mu.Lock()
	r.paths = append(r.paths, bin)
	n := len(r.paths)
	r.mu.Unlock()
	return r.fail(bin, n)
}

func answers(v string) (string, error) { return "nself-ci-agent " + v + " linux/amd64\n", nil }

func TestApplyLocalRunsTheNewBinaryBeforeAndAfterTheSwap(t *testing.T) {
	s, rel := newSim(t, "1.4.0"), newRelease(t, "1.5.0", 1, 1)
	rl := &runLog{fail: func(string, int) (string, error) { return answers("1.5.0") }}
	o := localOpts(s, "1.4.0", "1.5.0", rel.source())
	o.RunVersion = rl.run
	if _, err := ApplyLocal(context.Background(), o); err != nil || installed(t, s) != "1.5.0" {
		t.Fatalf("err %v version %s", err, installed(t, s))
	}
	if len(rl.paths) != 2 || rl.paths[0] == s.bin() || rl.paths[1] != s.bin() {
		t.Fatalf("ran %v, want the staged file first and the installed binary second", rl.paths)
	}
}

func TestApplyLocalInstallsNothingWhenTheNewBinaryDoesNotStart(t *testing.T) {
	for name, answer := range map[string]func() (string, error){
		"does not start":    func() (string, error) { return "", errors.New("exit status 1") },
		"reports another":   func() (string, error) { return answers("1.9.9") },
		"prints no version": func() (string, error) { return "hello\n", nil },
	} {
		s, rel := newSim(t, "1.4.0"), newRelease(t, "1.5.0", 1, 1)
		o := localOpts(s, "1.4.0", "1.5.0", rel.source())
		o.RunVersion = (&runLog{fail: func(string, int) (string, error) { return answer() }}).run
		_, err := ApplyLocal(context.Background(), o)
		if err == nil || !strings.HasPrefix(err.Error(), "E656:") || installed(t, s) != "1.4.0" {
			t.Fatalf("%s: err %v version %s", name, err, installed(t, s))
		}
		if _, serr := os.Lstat(s.bin() + PrevSuffix); serr == nil {
			t.Fatalf("%s: a refused upgrade left a .prev", name)
		}
	}
}

func TestApplyLocalRestoresWhenTheInstalledBinaryDoesNotStart(t *testing.T) {
	s, rel := newSim(t, "1.4.0"), newRelease(t, "1.5.0", 1, 1)
	rl := &runLog{fail: func(_ string, n int) (string, error) { // the staged copy runs, the installed one does not
		if n == 1 {
			return answers("1.5.0")
		}
		return "", errors.New("exec format error")
	}}
	o := localOpts(s, "1.4.0", "1.5.0", rel.source())
	o.RunVersion = rl.run
	res, err := ApplyLocal(context.Background(), o)
	if err == nil || !res.RolledBack || !strings.Contains(err.Error(), "does not start") || installed(t, s) != "1.4.0" {
		t.Fatalf("res %+v err %v version %s", res, err, installed(t, s))
	}
	if _, serr := os.Lstat(s.bin() + PrevVersionSuffix); serr == nil {
		t.Fatal("the marker of a swap that was undone is still there")
	}
}
