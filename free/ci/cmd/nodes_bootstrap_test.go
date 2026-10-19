package main

// Purpose: `nodes add --system` wiring (P7-NODE-06): the flag selects the installer, a conflicting
// --installer is refused, the guard hook fails an add that did not get the system layout, and the
// installer drives the bootstrap family against a fake node and reports the node as separated.
// Inputs:  fake lifecycle.Deps seams. Constraints: no network, no ssh.

import (
	"bytes"
	"context"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/nodes/bootstrap"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
)

func addFlags(t *testing.T, args ...string) (*flag.FlagSet, error) {
	t.Helper()
	fs := lifecycle.FlagSet("add")
	fs.SetOutput(io.Discard)
	return fs, fs.Parse(args)
}

func TestSystemFlagSelectsTheInstaller(t *testing.T) {
	fs, err := addFlags(t, "--system", "--slots", "4")
	if err != nil || flagString(fs, "installer") != systemInstaller || flagString(fs, "system") != "true" || flagString(fs, "slots") != "4" {
		t.Fatalf("installer=%q system=%q slots=%q err=%v", flagString(fs, "installer"), flagString(fs, "system"), flagString(fs, "slots"), err)
	}
	if fs, err = addFlags(t); err != nil || flagString(fs, "installer") != "" || flagString(fs, "system") != "false" || flagString(fs, "slots") != "2" {
		t.Fatalf("defaults: installer=%q system=%q slots=%q err=%v", flagString(fs, "installer"), flagString(fs, "system"), flagString(fs, "slots"), err)
	}
	if _, err = addFlags(t, "--installer", "other", "--system"); err == nil {
		t.Fatal("--system after another --installer was accepted")
	}
	if fs, err = addFlags(t, "--installer", "system", "--system=true"); err != nil || flagString(fs, "installer") != "system" {
		t.Fatalf("same installer twice: %v", err)
	}
	if fs, err = addFlags(t, "--system=false"); err != nil || flagString(fs, "installer") != "" {
		t.Fatalf("--system=false: installer=%q err=%v", flagString(fs, "installer"), err)
	}
}

func TestSystemGuardFailsAnAddThatSkippedTheInstaller(t *testing.T) {
	fs, _ := addFlags(t, "--system")
	if err := systemGuard(context.Background(), lifecycle.HookEvent{NodeID: "guard-1", Flags: fs}); bootstrap.ReasonOf(err) != bootstrap.ReasonUsage {
		t.Fatalf("guard: %v", err)
	}
	installed.Store("guard-2", true)
	if err := systemGuard(context.Background(), lifecycle.HookEvent{NodeID: "guard-2", Flags: fs}); err != nil {
		t.Fatalf("an installed node was refused: %v", err)
	}
	plain, _ := addFlags(t)
	if err := systemGuard(context.Background(), lifecycle.HookEvent{NodeID: "guard-3", Flags: plain}); err != nil {
		t.Fatalf("a user-mode add was refused: %v", err)
	}
}

type fakeSource struct{ path string }

func (f fakeSource) Fetch(context.Context, lifecycle.Platform) (string, func(), error) {
	return f.path, nil, nil
}

type bareSeparation struct{}

func (bareSeparation) Detect(context.Context, func(context.Context, ...string) (string, error), lifecycle.Platform) lifecycle.SeparationFacts {
	return lifecycle.SeparationFacts{Reason: "probe_unavailable"}
}

func fakeDeps(t *testing.T, osName string) (*lifecycle.Deps, *[][]string, *bytes.Buffer) {
	t.Helper()
	var calls [][]string
	var errb bytes.Buffer
	d := &lifecycle.Deps{Stderr: &errb, Separation: bareSeparation{},
		Run: func(_ context.Context, _ remote.Target, argv ...string) (string, error) {
			calls = append(calls, argv)
			switch strings.Join(argv, " ") {
			case "uname -s":
				return osName, nil
			case "cat /etc/os-release":
				return "ID=debian\n", nil
			case "id -u":
				return "0", nil
			}
			return "", nil
		},
		Copy: func(_ context.Context, _ remote.Target, local, remotePath string) error {
			calls = append(calls, []string{"copy", filepath.Base(local), remotePath})
			return nil
		}}
	return d, &calls, &errb
}

func TestSystemInstallBootstrapsAndKeepsTheNodeUnseparated(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "agent")
	if err := os.WriteFile(bin, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	d, calls, _ := fakeDeps(t, "Linux")
	fs, _ := addFlags(t, "--system", "--slots", "3")
	req := lifecycle.InstallRequest{NodeID: "sys-1", Platform: lifecycle.Platform{OS: "linux", Arch: "amd64"}, Dir: ".nself/ci-agent", Source: fakeSource{bin}, Flags: fs}
	if err := systemInstall(context.Background(), d, req); err != nil {
		t.Fatal(err)
	}
	all := ""
	for _, c := range *calls {
		all += strings.Join(c, " ") + "\n"
	}
	for _, want := range []string{"for n in 1 2 3;", "useradd --system --user-group", "copy agent .nself-ci-agent.upload", "ln -sf /opt/nself-ci-agent/bin/nself-ci-agent '.nself/ci-agent/bin/nself-ci-agent'"} {
		if !strings.Contains(all, want) {
			t.Errorf("the node never saw %q:\n%s", want, all)
		}
	}
	if facts := d.Separation.Detect(context.Background(), nil, lifecycle.Platform{OS: "linux"}); facts.UIDSeparation {
		t.Fatalf("a node whose jobs do not run through the shim was marked separated: %+v", facts)
	}
	if _, ok := installed.Load("sys-1"); !ok {
		t.Fatal("the guard would refuse a finished install")
	}
	installed.Delete("sys-1")
}

func TestSystemInstallWithoutAFamilyPrintsBrewForDarwin(t *testing.T) {
	if _, ok := bootstrap.Lookup("darwin"); ok {
		t.Skip("a darwin family is registered")
	}
	bin := filepath.Join(t.TempDir(), "agent")
	_ = os.WriteFile(bin, []byte("x"), 0o755)
	d, _, errb := fakeDeps(t, "Darwin")
	fs, _ := addFlags(t, "--system")
	err := systemInstall(context.Background(), d, lifecycle.InstallRequest{NodeID: "mac-1", Platform: lifecycle.Platform{OS: "darwin", Arch: "arm64"}, Source: fakeSource{bin}, Flags: fs})
	if bootstrap.ReasonOf(err) != bootstrap.ReasonUnsupported || !strings.Contains(errb.String(), "brew install git") {
		t.Fatalf("err=%v stderr=%q", err, errb.String())
	}
	if facts := d.Separation.Detect(context.Background(), nil, lifecycle.Platform{OS: "darwin"}); facts.UIDSeparation {
		t.Fatal("an unsupported node was marked separated")
	}
}

func TestSystemInstallNeedsTheBinary(t *testing.T) {
	d, _, _ := fakeDeps(t, "Linux")
	if err := systemInstall(context.Background(), d, lifecycle.InstallRequest{NodeID: "x"}); bootstrap.ReasonOf(err) != bootstrap.ReasonUsage {
		t.Fatalf("err = %v", err)
	}
}

func TestRunnerPinFromTheEmbeddedTable(t *testing.T) {
	pin, err := runnerPin()
	if err != nil {
		t.Fatal(err)
	}
	if pin.Version == "" || pin.Sources["amd64"].SHA256 == "" || pin.Sources["arm64"].SHA256 == "" {
		t.Fatalf("pin = %+v", pin)
	}
	for arch, s := range pin.Sources {
		if !strings.HasPrefix(s.URL, "https://github.com/actions/runner/releases/download/v"+pin.Version+"/") || !strings.Contains(s.URL, pin.Version) {
			t.Errorf("%s url %q is not the pinned release", arch, s.URL)
		}
	}
}

func TestSystemInstallRefusesBadSlots(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "agent")
	_ = os.WriteFile(bin, []byte("x"), 0o755)
	for _, slots := range []string{"0", "-1", "65", "x"} {
		d, calls, _ := fakeDeps(t, "Linux")
		fs, _ := addFlags(t, "--system", "--slots", slots)
		err := systemInstall(context.Background(), d, lifecycle.InstallRequest{NodeID: "bad", Platform: lifecycle.Platform{OS: "linux", Arch: "amd64"}, Dir: ".nself/ci-agent", Source: fakeSource{bin}, Flags: fs})
		if bootstrap.ReasonOf(err) != bootstrap.ReasonUsage {
			t.Errorf("--slots %s: %v", slots, err)
		}
		for _, c := range *calls {
			if len(c) > 2 && c[len(c)-3] == "sh" && strings.Contains(c[len(c)-1], "useradd") {
				t.Errorf("--slots %s created users", slots)
			}
		}
	}
}
