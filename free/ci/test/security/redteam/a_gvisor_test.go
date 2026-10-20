//go:build security

package redteam

// Purpose: red team attacks on the gVisor runtime flag deny list and the real-start probe (TM-06).
//   - A34: runsc runtime arguments in every spelling the deny list could miss (split value, single dash,
//     case, network host, shared file access, TESTONLY flags, platform last-wins);
//   - A35: a probe container that starts but does not print the gVisor banner, or prints it mid-line,
//     must not make the sandboxed-container level available.
// Inputs:  gvisor.New with a fake docker query runner and a fake probe starter.
// Outputs: pass (control held) or fail (attack worked).
// Constraints: no docker, no runsc; not-attackable-locally: an actual sandbox escape needs the Linux kernel.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/isolation/gvisor"
)

type aDockerQ struct{ runtimes string }

func (q aDockerQ) Query(_ context.Context, _ string, args ...string) ([]byte, error) {
	if strings.Contains(strings.Join(args, " "), ".Runtimes") {
		return []byte(q.runtimes), nil
	}
	return []byte(`{"ServerVersion":"27.0.1","MemoryLimit":true,"PidsLimit":true,"CPUCfsQuota":true,"SecurityOptions":["name=seccomp,profile=builtin"]}`), nil
}

type aStarter struct {
	out string
	err error
}

func (s aStarter) Banner(context.Context, string, string, string) ([]byte, error) {
	return []byte(s.out), s.err
}
func (s aStarter) Version(context.Context, string) (string, error) { return "release-1", nil }

func aRuntimes(path string, args ...string) string {
	b, _ := json.Marshal(map[string]any{"runsc": map[string]any{"path": path, "runtimeArgs": args}})
	return string(b)
}

func aGVisor(table string, st aStarter) gvisor.Options {
	return gvisor.Options{Runner: aDockerQ{table}, Starter: st, HasKVM: func() bool { return false }, Image: "alpine"}
}

const aBanner = "[    0.000000] Starting gVisor...\n"

func TestA34_RuntimeArgumentSpellings(t *testing.T) {
	ctx := context.Background()
	if av := gvisor.New(aGVisor(aRuntimes("/usr/local/bin/runsc", "--platform=systrap"), aStarter{out: aBanner})).Probe(ctx); !av.Available() {
		t.Fatalf("control: a plain systrap entry must be available: %+v", av)
	}
	hostile := [][]string{
		{"--host-uds=all"}, {"--host-uds", "all"}, {"-host-uds=open"}, {"--net-raw"}, {"--net-raw=true"}, {"--ignore-cgroups"}, {"--ignore-cgroups=false"},
		{"--network=host"}, {"--network", "host"}, {"-network=host"}, {"--file-access=shared"}, {"--file-access", "shared"}, {"--TESTONLY-unsafe-nonroot"},
		{"--testonly-allow-packet-endpoint-write"}, {"--rootless"}, {"--allow-flag-override"}, {"--profile"}, {"--nvproxy=true"}, {"--host-fifo=open"},
		{"--fsgofer-host-uds"}, {"--platform=kvm"}, {"--platform=ptrace", "--platform=kvm"}, {"--platform="}, {"--platform=Systrap"}, {"--platform", ""},
	}
	for _, args := range hostile {
		av := gvisor.New(aGVisor(aRuntimes("/usr/local/bin/runsc", args...), aStarter{out: aBanner})).Probe(ctx)
		if av.Available() {
			t.Errorf("runsc arguments %q left sandboxed-container available", args)
		}
	}
	for name, table := range map[string]string{
		"path is runc":        aRuntimes("/usr/bin/runc"),
		"empty path":          aRuntimes(""),
		"foreign key":         `{"runsc":{"path":"/usr/bin/runsc","runtimeArgs":[],"features":{"x":1}}}`,
		"not json":            `runsc`,
		"runsc missing":       `{"runc":{"path":"/usr/bin/runc"}}`,
		"runtimeArgs a map":   `{"runsc":{"path":"/usr/bin/runsc","runtimeArgs":{"a":"b"}}}`,
		"upper case key name": `{"RUNSC":{"path":"/usr/bin/runsc"}}`,
	} {
		if av := gvisor.New(aGVisor(table, aStarter{out: aBanner})).Probe(ctx); av.Available() {
			t.Errorf("%s: sandboxed-container available", name)
		}
	}
}

func TestA35_BannerIsTheProof(t *testing.T) {
	ctx := context.Background()
	table := aRuntimes("/usr/local/bin/runsc")
	if av := gvisor.New(aGVisor(table, aStarter{out: aBanner})).Probe(ctx); !av.Available() { // positive control: the real banner makes it available
		t.Fatalf("control: a runtime that printed the gVisor banner is not available: %+v", av)
	}
	for name, st := range map[string]aStarter{
		"no banner":                {out: "hello\n"},
		"empty":                    {out: ""},
		"start error":              {err: errors.New("exec: runsc: not found")},
		"banner mid line":          {out: "echo Starting gVisor... please\n"},
		"banner as a quoted value": {out: "cmdline: 'Starting gVisor...' x\n"},
		"banner with prefix text":  {out: "kernel: Starting gVisor...\n"},
		"lower case":               {out: "starting gvisor...\n"},
	} {
		if av := gvisor.New(aGVisor(table, st)).Probe(ctx); av.Available() {
			t.Errorf("%s: a container that was not shown to run under gVisor made the level available", name)
		}
	}
}
