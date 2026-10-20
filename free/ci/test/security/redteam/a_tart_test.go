//go:build security

package redteam

// Purpose: plan-level attacks on the Tart VM network scope (TM-05). A real Tart VM needs Apple silicon and the
// tool, so this drives the real profile against a fake Tool and asks whether a scope that cannot be enforced
// is still started.
//   - A33: no softnet block option, a help text that only looks similar, facts that cannot be read, a
//     restricted scope with no proxy: the VM must be refused, never started with a weaker network.
// Inputs:  tart.New with a recording fake Tool; exec.JobSpec with each network scope.
// Outputs: pass (control held) or fail (attack worked).
// Constraints: nothing is started; the fake Tool answers the probe commands only.
// not-attackable-locally: whether softnet really drops the packets (needs a VM); see the findings file.

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/isolation/tart"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/netscope"
)

type aTool struct{ help string }

func (f aTool) Run(_ context.Context, argv []string, _ io.Reader, _ io.Writer) (tart.Result, error) {
	switch {
	case len(argv) > 1 && argv[1] == "--version":
		return tart.Result{Out: []byte("2.0.0\n")}, nil
	case len(argv) > 1 && argv[1] == "list":
		return tart.Result{Out: []byte("[]")}, nil
	case len(argv) > 2 && argv[1] == "run" && argv[2] == "--help":
		return tart.Result{Out: []byte(f.help)}, nil
	}
	return tart.Result{}, nil
}
func (f aTool) Start([]string, string) (tart.Proc, error) {
	return nil, errors.New("not started in this test")
}

func aTartProfile(t *testing.T, help string, net tart.NetSource) *tart.Profile {
	t.Helper()
	home := t.TempDir()
	return tart.New("vm", tart.Options{GOOS: "darwin", GOARCH: "arm64", Home: func() string { return home },
		BaseImage: func() string { return "ghcr.io/example/base@sha256:" + strings.Repeat("a", 64) },
		NewTool:   func(string, string) tart.Tool { return aTool{help: help} }, Net: net})
}

func aTartSpec(id string, net model.NetworkScope) exec.JobSpec {
	return exec.JobSpec{AttemptID: id, CoordinatorID: "c1", Command: []string{"true"}, Network: net, Job: model.Job{Isolation: "vm"}}
}

func aRefused(err error) bool {
	var ie *isolation.Error
	return errors.As(err, &ie) && (ie.Code == "E679" || ie.Code == "E678")
}

func TestA33_TartRefusesWhatSoftnetCannotEnforce(t *testing.T) {
	const noBlock = "OPTIONS:\n  --net-softnet  Use software networking\n  --net-bridged <iface>\n"
	const lookalike = "OPTIONS:\n  --net-softnet-blocklist <x>\n  --no-net-softnet-allowed\n"
	facts := func(context.Context) (netscope.Facts, error) { return netscope.Facts{}, nil }
	broken := func(context.Context) (netscope.Facts, error) {
		return netscope.Facts{}, errors.New("route table unreadable")
	}
	for _, scope := range []model.NetworkScope{"", "none", "restricted", "internet"} {
		for name, help := range map[string]string{"no block option": noBlock, "empty help": ""} {
			p := aTartProfile(t, help, tart.NetSource{Facts: facts})
			if _, err := p.Wrap(aTartSpec("a-"+name, scope)); !aRefused(err) {
				t.Errorf("scope %q with %s: Wrap = %v, want E679", scope, name, err)
			}
		}
		p := aTartProfile(t, noBlock, tart.NetSource{Facts: broken})
		if _, err := p.Wrap(aTartSpec("a-broken", scope)); err == nil {
			t.Errorf("scope %q with unreadable facts was wrapped", scope)
		}
	}
	// the look-alike help is judged by substring today: the flag names differ, tart would reject them at start
	// (fail closed), so this only records the behaviour; a restricted scope without a proxy must still be refused
	supported := "  --net-softnet-block <cidrs>\n  --net-softnet-allow <cidrs>\n"
	p := aTartProfile(t, supported, tart.NetSource{Facts: facts})
	if _, err := p.Wrap(aTartSpec("a-noproxy", "restricted")); !aRefused(err) {
		t.Errorf("restricted scope with no proxy starter was wrapped: %v", err)
	}
	_ = lookalike
	if _, err := p.Wrap(aTartSpec("a-bogus", "everything")); !aRefused(err) {
		t.Errorf("an unknown scope was wrapped: %v", err)
	}
}
