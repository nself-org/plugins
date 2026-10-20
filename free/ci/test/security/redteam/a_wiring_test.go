//go:build security

package redteam

// Purpose: red team checks that the isolation and admission controls are wired into the production path
// (TM-05, TM-07, TM-08, TM-10, TM-11). A control that is correct but never called protects nothing.
//   - A11/A12: the executor the node agent builds for a lease has no Isolator, so the hardened container
//     profile, the gVisor profile and the network scope planner never apply to an agent-run job;
//   - A14..A16: the world builder never fills World.PreviousIsolation, World.Network or Runner.Decl.Running,
//     the three inputs of the retry floor, the NoLANRoute proof and the co-tenancy rule.
// Inputs:  the repository sources (read only) and exec.Executor built the way cmd/agent builds it.
// Outputs: pass (wired) or fail (attack worked: the control is dead code on the production path).
// Constraints: static source checks read files relative to this package; a fake docker records argv.

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/model"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

const aModuleRoot = "../../.." // free/ci

// aSources returns the non-test Go sources under dirs (relative to free/ci) with their text.
func aSources(t *testing.T, dirs ...string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, d := range dirs {
		root := filepath.Join(aModuleRoot, d)
		err := filepath.WalkDir(root, func(p string, e fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if e.IsDir() && (e.Name() == "vendor" || e.Name() == "testdata") {
				return filepath.SkipDir
			}
			if !e.IsDir() && strings.HasSuffix(p, ".go") && !strings.HasSuffix(p, "_test.go") {
				b, rerr := os.ReadFile(p)
				if rerr != nil {
					return rerr
				}
				out[p] = string(b)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return out
}

func aAnyContains(src map[string]string, skipDirs []string, needles ...string) (string, bool) {
	for p, text := range src {
		skip := false
		for _, d := range skipDirs {
			if strings.Contains(filepath.ToSlash(p), d) {
				skip = true
			}
		}
		if skip {
			continue
		}
		for _, n := range needles {
			if strings.Contains(text, n) {
				return p, true
			}
		}
	}
	return "", false
}

// A11: the node agent executes leases through an Executor with no Isolator (Isolation nil means the
// baseline isolator: process, or the unhardened legacy container).
func TestA11_AgentExecutorHasAnIsolator(t *testing.T) {
	text, err := os.ReadFile(filepath.Join(aModuleRoot, "cmd/agent/exec.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(text), "\n") {
		if strings.Contains(line, "ciexec.Executor{") {
			if !strings.Contains(line, "Isolation:") {
				t.Errorf("cmd/agent/exec.go builds the lease executor without an Isolator: %s", strings.TrimSpace(line))
			}
			return
		}
	}
	t.Fatal("no Executor literal found in cmd/agent/exec.go")
}

// A12: what an Executor built like the agent's does with a container job: the legacy container carries none
// of the hardened flags, and a sandboxed-container job is refused (fail closed, E611).
func TestA12_AgentBuiltExecutorContainerHardening(t *testing.T) {
	runs := aFakeDocker(t)
	iso := cirun.Hooks{}.Resolved().Isolation // what cmd/agent/exec.go installs on its lease executor
	ex := &exec.Executor{Env: exec.AllowlistEnv{}, NewRedactor: exec.NewOutputRedactor, Grace: time.Second, Isolation: iso}
	run := func(level model.Isolation, net model.NetworkScope, id string) exec.AttemptResult {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return ex.Run(ctx, exec.JobSpec{AttemptID: id, CoordinatorID: "c1", Home: t.TempDir(), Network: net,
			Job: model.Job{Isolation: level, Container: &model.ContainerConfig{Image: "alpine"}}, Command: []string{"true"}})
	}
	sb := run("sandboxed-container", "none", "rt-sb")
	var ce *exec.Error
	var ie *isolation.Error // the registered Isolator refuses a level no profile can provide (E678); a bare executor refuses it with E611
	if !(errors.As(sb.Err, &ce) && ce.Code == "E611" || errors.As(sb.Err, &ie) && ie.Code == "E678") || len(runs()) != 0 {
		t.Errorf("sandboxed-container on an agent-built executor must fail closed (E611 or E678): err=%v runs=%d", sb.Err, len(runs()))
	}
	if res := run("container", "none", "rt-c1"); res.Err != nil || len(runs()) < 1 { // the hardened profile adds a workspace-cleanup run after the job
		t.Fatalf("legacy container run: err=%v runs=%v", res.Err, runs())
	}
	args := strings.Join(runs()[0], " ")
	for _, want := range []string{"--cap-drop=ALL", "no-new-privileges", "--user", "--pids-limit", "--network"} {
		if !strings.Contains(args, want) {
			t.Errorf("container planned by an agent-built executor lacks %s: %s", want, args)
		}
	}
}

// A14: the retry isolation floor reads World.PreviousIsolation; the world builder must fill it.
func TestA14_RetryFloorInputIsFilled(t *testing.T) {
	src := aSources(t, "internal", "cmd")
	if p, ok := aAnyContains(src, []string{"internal/sched/"}, "PreviousIsolation"); !ok {
		t.Error("nothing outside internal/sched ever sets World.PreviousIsolation, so a retry or re-placement has no isolation floor in production")
	} else {
		t.Logf("PreviousIsolation is set in %s", p)
	}
}

// A15: the co-tenancy rule reads Declaration.Running; some production code must say what runs on a node.
func TestA15_CotenancyInputIsFilled(t *testing.T) {
	src := aSources(t, "internal", "cmd")
	if p, ok := aAnyContains(src, []string{"internal/trust/"}, "SecretBearing", "trust.Running{", "Running.Untrusted"); !ok {
		t.Error("nothing outside internal/trust ever sets Declaration.Running, so isolation.cotenancy can never fire in production")
	} else {
		t.Logf("Running is set in %s", p)
	}
}

// A16: the NoLANRoute proof reads World.Network; a contributor must fill it from the probe.
func TestA16_NetworkProofInputIsFilled(t *testing.T) {
	src := aSources(t, "internal", "cmd")
	if p, ok := aAnyContains(src, []string{"internal/sched/"}, "NetworkFact{", `Field: "Network"`); !ok {
		t.Error("nothing outside internal/sched ever fills World.Network, so no runner is ever proven free of a LAN route and untrusted work can only go to hosted capacity")
	} else {
		t.Logf("World.Network is filled in %s", p)
	}
}
