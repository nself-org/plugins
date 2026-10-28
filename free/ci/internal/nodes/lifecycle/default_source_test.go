package lifecycle

// Purpose: tests of the default install source seam (P7-NODE-15): `nodes add` without --agent-binary uses the registered default
//          source only when the node has no agent yet, and --agent-binary still wins.
// Inputs:  the unit kit (fake ssh, scp, probe) and a counting fake source.
// Constraints: the real Release source lives in internal/agent/install; this package cannot import it (cycle), so cmd's test
//          covers the wiring end to end.

import (
	"context"
	"debug/elf"
	"strings"
	"sync/atomic"
	"testing"
)

type countingSource struct {
	path  string
	calls atomic.Int32
}

func (c *countingSource) Fetch(context.Context, Platform) (string, func(), error) {
	c.calls.Add(1)
	return c.path, nil, nil
}

func TestDefaultSourceInstallsWhenTheNodeHasNoAgent(t *testing.T) {
	resetExtensions()
	t.Cleanup(resetExtensions)
	k := newKit(t)
	src := &countingSource{path: elfFixture(t, elf.EM_X86_64)}
	RegisterDefaultSource(func() AgentSource { return src })
	k.failRun["test"] = errBoom // `test -x` fails: no agent on the node
	if _, err := k.d.Add(context.Background(), AddOptions{Dest: "ci@fresh", HostKey: fpA, Trust: DefaultTrust()}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if src.calls.Load() != 1 || len(k.copies) != 1 || k.copies[0] != src.path {
		t.Fatalf("default source calls %d, copies %v", src.calls.Load(), k.copies)
	}
}

func TestDefaultSourceIsSkippedWhenAnAgentIsInstalled(t *testing.T) {
	resetExtensions()
	t.Cleanup(resetExtensions)
	k := newKit(t)
	src := &countingSource{path: elfFixture(t, elf.EM_X86_64)}
	RegisterDefaultSource(func() AgentSource { return src })
	if _, err := k.d.Add(context.Background(), AddOptions{Dest: "ci@have", HostKey: fpA, Trust: DefaultTrust()}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if src.calls.Load() != 0 || len(k.copies) != 0 {
		t.Fatalf("an installed agent must be used as it is: calls %d copies %v", src.calls.Load(), k.copies)
	}
}

func TestAgentBinaryFlagBeatsTheDefaultSource(t *testing.T) {
	resetExtensions()
	t.Cleanup(resetExtensions)
	k := newKit(t)
	src := &countingSource{path: "/never/used"}
	RegisterDefaultSource(func() AgentSource { return src })
	local := elfFixture(t, elf.EM_X86_64)
	if _, err := k.d.Add(context.Background(), AddOptions{Dest: "ci@flag", HostKey: fpA, Source: LocalFile{Path: local}, Trust: DefaultTrust()}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if src.calls.Load() != 0 || len(k.copies) != 1 || k.copies[0] != local {
		t.Fatalf("--agent-binary must win: calls %d copies %v", src.calls.Load(), k.copies)
	}
}

func TestDefaultSourceErrorFailsTheAddAndLeavesNoNode(t *testing.T) {
	resetExtensions()
	t.Cleanup(resetExtensions)
	k := newKit(t)
	RegisterDefaultSource(func() AgentSource { return failSource{} })
	k.failRun["test"] = errBoom
	_, err := k.d.Add(context.Background(), AddOptions{Dest: "ci@bad", HostKey: fpA, Trust: DefaultTrust()})
	if err == nil || CodeOf(err) != "E665" {
		t.Fatalf("want the source's E665, got %v", err)
	}
	if n, _ := k.d.Registry.List(context.Background()); len(n) != 0 {
		t.Fatalf("a failed install must register nothing: %v", n)
	}
}

type failSource struct{}

func (failSource) Fetch(context.Context, Platform) (string, func(), error) {
	return "", nil, errCoded("E665: no_trusted_key: pass --agent-binary <path>")
}

type errCoded string

func (e errCoded) Error() string { return string(e) }

func TestRegisterDefaultSourceTwicePanics(t *testing.T) {
	resetExtensions()
	t.Cleanup(resetExtensions)
	RegisterDefaultSource(func() AgentSource { return nil })
	defer func() {
		if r := recover(); r == nil || !strings.Contains(r.(string), "default agent source") {
			t.Fatalf("want a panic on the second registration, got %v", r)
		}
	}()
	RegisterDefaultSource(func() AgentSource { return nil })
}

type pinnedSource struct {
	countingSource
	pin string
}

func (p *pinnedSource) PinnedVersion() string { return p.pin }

// D-1223: an agent already on the node that is not the pinned release is reported, not silently accepted.
func TestInstalledAgentThatDiffersFromThePinIsWarnedAbout(t *testing.T) {
	cases := []struct {
		name, pin string
		warn      bool
	}{{"older than pin", "1.6.0", true}, {"same as pin", "1.5.0", false}, {"no pin", "", false}}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resetExtensions()
			t.Cleanup(resetExtensions)
			k := newKit(t)
			src := &pinnedSource{countingSource{path: elfFixture(t, elf.EM_X86_64)}, c.pin}
			RegisterDefaultSource(func() AgentSource { return src })
			r, err := k.d.Add(context.Background(), AddOptions{Dest: "ci@have", HostKey: fpA, Trust: DefaultTrust()})
			if err != nil {
				t.Fatalf("add: %v", err)
			}
			if src.calls.Load() != 0 || len(k.copies) != 0 {
				t.Fatalf("the installed agent must still be used as it is: calls %d copies %v", src.calls.Load(), k.copies)
			}
			if got := len(r.Warnings) > 0; got != c.warn {
				t.Fatalf("warnings %v, want warning=%v", r.Warnings, c.warn)
			}
			if c.warn && (!strings.Contains(r.Warnings[0], "1.5.0") || !strings.Contains(r.Warnings[0], c.pin)) {
				t.Fatalf("the warning must name both versions: %q", r.Warnings[0])
			}
			if c.warn && !strings.Contains(r.Warnings[0], "`nself ci nodes upgrade have`") {
				t.Fatalf("the warning must offer `nodes upgrade` for the node: %q", r.Warnings[0])
			}
		})
	}
}

func TestAgentBinaryFlagSkipsThePinWarning(t *testing.T) {
	resetExtensions()
	t.Cleanup(resetExtensions)
	k := newKit(t)
	RegisterDefaultSource(func() AgentSource { return &pinnedSource{pin: "9.9.9"} })
	r, err := k.d.Add(context.Background(), AddOptions{Dest: "ci@flag", HostKey: fpA, Source: LocalFile{Path: elfFixture(t, elf.EM_X86_64)}, Trust: DefaultTrust()})
	if err != nil || len(r.Warnings) != 0 {
		t.Fatalf("err %v warnings %v", err, r.Warnings)
	}
}
