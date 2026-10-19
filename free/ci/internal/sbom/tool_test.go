package sbom_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/sbom"
	"github.com/nself-org/plugins/free/ci/internal/toolchain"
)

func TestParseVersion(t *testing.T) {
	good := "Application:   syft\nVersion:       1.54.1\nBuildDate:     x\n"
	if v, ok := sbom.ParseVersion(good); !ok || v != "1.54.1" {
		t.Fatalf("got %q %v", v, ok)
	}
	if v, ok := sbom.ParseVersion("Version: v2.0.0\n"); !ok || v != "2.0.0" {
		t.Fatalf("v prefix: %q %v", v, ok)
	}
	for _, bad := range []string{"", "syft 1.54.1", "NotVersion: 1.0"} {
		if _, ok := sbom.ParseVersion(bad); ok {
			t.Errorf("%q parsed", bad)
		}
	}
}

func TestEmbeddedPinHasSyftRowWithChecksums(t *testing.T) {
	pin, err := sbom.EmbeddedPin()
	if err != nil {
		t.Fatal(err)
	}
	if pin.Version == "" || len(pin.Sources) == 0 {
		t.Fatalf("pin = %+v", pin)
	}
	for _, os := range []string{"linux", "darwin"} {
		for _, arch := range []string{"amd64", "arm64"} {
			s, ok := pin.Source(os, arch)
			if !ok || len(s.SHA256) != 64 || len(s.BinarySHA256) != 64 || !strings.HasPrefix(s.URL, "https://") {
				t.Errorf("%s/%s source = %+v", os, arch, s)
			}
		}
	}
	if _, err := sbom.PinFrom(toolchain.Versions{Version: 1}); !sbom.IsReason(err, sbom.ReasonToolMissing) {
		t.Fatalf("missing row: %v", err)
	}
}

func TestToolPath(t *testing.T) {
	if got := sbom.ToolPath("/h", "1.2.3"); got != filepath.FromSlash("/h/tools/syft-1.2.3/syft") {
		t.Fatal(got)
	}
}

func TestResolveProcessUsesOnlyTheInstalledSyft(t *testing.T) {
	ctx := context.Background()
	t.Run("installed at the pin", func(t *testing.T) {
		home := t.TempDir()
		p := installedSyft(t, home)
		got, err := sbom.Resolve(ctx, &fakeRunner{versions: map[string]string{p: pinVersion}}, home, sbom.ModeProcess, testPin())
		if err != nil || got != p {
			t.Fatalf("got %q %v", got, err)
		}
	})
	t.Run("installed binary reports another version", func(t *testing.T) {
		home := t.TempDir()
		p := installedSyft(t, home)
		_, err := sbom.Resolve(ctx, &fakeRunner{versions: map[string]string{p: "1.0.0"}}, home, sbom.ModeProcess, testPin())
		if !sbom.IsReason(err, sbom.ReasonToolVersion) || !strings.Contains(err.Error(), sbom.InstallCommand) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("installed binary does not run", func(t *testing.T) {
		home := t.TempDir()
		installedSyft(t, home)
		_, err := sbom.Resolve(ctx, &fakeRunner{versions: map[string]string{}}, home, sbom.ModeProcess, testPin())
		if !sbom.IsReason(err, sbom.ReasonToolMissing) || !strings.Contains(err.Error(), sbom.InstallCommand) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("nothing installed, nothing on PATH", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		_, err := sbom.Resolve(ctx, &fakeRunner{}, t.TempDir(), sbom.ModeProcess, testPin())
		if !sbom.IsReason(err, sbom.ReasonToolMissing) || !strings.Contains(err.Error(), sbom.InstallCommand) {
			t.Fatalf("got %v", err)
		}
	})
	onPath := func(t *testing.T) string {
		dir := t.TempDir()
		p := filepath.Join(dir, "syft")
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", dir)
		return p
	}
	t.Run("only a PATH syft: reported, never run", func(t *testing.T) {
		p := onPath(t)
		fr := &fakeRunner{versions: map[string]string{p: pinVersion}}
		_, err := sbom.Resolve(ctx, fr, t.TempDir(), sbom.ModeProcess, testPin())
		if !sbom.IsReason(err, sbom.ReasonToolVersion) || !strings.Contains(err.Error(), sbom.InstallCommand) {
			t.Fatalf("got %v", err)
		}
		if len(fr.cmds) != 0 {
			t.Fatalf("a PATH syft was executed: %v", fr.cmds)
		}
	})
	t.Run("a directory or non-executable at the install path is not an install", func(t *testing.T) {
		home := t.TempDir()
		p := sbom.ToolPath(home, pinVersion)
		if err := os.MkdirAll(p, 0o755); err != nil { // a directory named syft
			t.Fatal(err)
		}
		t.Setenv("PATH", t.TempDir())
		if _, err := sbom.Resolve(ctx, &fakeRunner{}, home, sbom.ModeProcess, testPin()); !sbom.IsReason(err, sbom.ReasonToolMissing) {
			t.Fatalf("got %v", err)
		}
	})
}

func TestResolveContainerUsesPathSyftAtThePin(t *testing.T) {
	ctx := context.Background()
	got, err := sbom.Resolve(ctx, &fakeRunner{versions: map[string]string{"syft": pinVersion}}, t.TempDir(), sbom.ModeContainer, testPin())
	if err != nil || got != "syft" {
		t.Fatalf("got %q %v", got, err)
	}
	_, err = sbom.Resolve(ctx, &fakeRunner{versions: map[string]string{"syft": "1.0.0"}}, t.TempDir(), sbom.ModeContainer, testPin())
	if !sbom.IsReason(err, sbom.ReasonToolVersion) {
		t.Fatalf("version drift: %v", err)
	}
	_, err = sbom.Resolve(ctx, &fakeRunner{versions: map[string]string{}}, t.TempDir(), sbom.ModeContainer, testPin())
	if !sbom.IsReason(err, sbom.ReasonToolMissing) {
		t.Fatalf("absent: %v", err)
	}
}

// TestResolveNeverExecutesAPlantedPathSyft uses a real runner: the planted
// syft would create a marker file if anything ran it.
func TestResolveNeverExecutesAPlantedPathSyft(t *testing.T) {
	dir, marker := t.TempDir(), filepath.Join(t.TempDir(), "ran")
	script := "#!/bin/sh\n: > '" + marker + "'\necho 'Version: 1.54.1'\n"
	if err := os.WriteFile(filepath.Join(dir, "syft"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":/usr/bin:/bin") // the executor itself needs sh
	// control: the fixture does leave the marker when something runs it
	if _, err := execRunner(t, t.TempDir()).Run(context.Background(), sbom.Cmd{Argv: []string{filepath.Join(dir, "syft"), "version"}}); err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(marker); statErr != nil {
		t.Fatalf("positive control: the planted script did not leave its marker: %v", statErr)
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	_, err := sbom.Resolve(context.Background(), execRunner(t, t.TempDir()), t.TempDir(), sbom.ModeProcess, testPin())
	if !sbom.IsReason(err, sbom.ReasonToolVersion) {
		t.Fatalf("got %v", err)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatal("the planted PATH syft was executed")
	}
}

func TestResolveHomeIsAbsoluteAndDefaulted(t *testing.T) {
	if h, err := sbom.ResolveHome("/x/y/../ci"); err != nil || h != "/x/ci" {
		t.Fatalf("explicit: %q %v", h, err)
	}
	t.Setenv("NSELF_CI_HOME", "/env/ci")
	if h, err := sbom.ResolveHome(""); err != nil || h != "/env/ci" {
		t.Fatalf("env: %q %v", h, err)
	}
	t.Setenv("NSELF_CI_HOME", "")
	t.Setenv("HOME", "/users/u")
	if h, err := sbom.ResolveHome(""); err != nil || h != "/users/u/.nself/ci" {
		t.Fatalf("default: %q %v", h, err)
	}
	for _, bad := range []string{"rel", ".", "tools/..", "../x"} {
		if _, err := sbom.ResolveHome(bad); !sbom.IsReason(err, sbom.ReasonToolMissing) {
			t.Errorf("%q: %v", bad, err)
		}
	}
	t.Setenv("NSELF_CI_HOME", "relative")
	if _, err := sbom.ResolveHome(""); !sbom.IsReason(err, sbom.ReasonToolMissing) {
		t.Fatalf("relative env: %v", err)
	}
}

func TestResolveRefusesRelativeHomesAndPlantedTools(t *testing.T) {
	src := t.TempDir()
	planted := filepath.Join(src, "tools", "syft-"+pinVersion, "syft")
	if err := os.MkdirAll(filepath.Dir(planted), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(planted, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(src)
	t.Setenv("NSELF_CI_HOME", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	fr := &fakeRunner{versions: map[string]string{planted: pinVersion, "tools/syft-" + pinVersion + "/syft": pinVersion}}
	for _, home := range []string{"", ".", "rel"} {
		if _, err := sbom.Resolve(context.Background(), fr, home, sbom.ModeProcess, testPin()); !sbom.IsReason(err, sbom.ReasonToolMissing) {
			t.Errorf("home %q: %v", home, err)
		}
	}
	if len(fr.cmds) != 0 {
		t.Fatalf("a planted tool was run: %v", fr.cmds)
	}
}

func TestResolveRefusesUntrustworthyInstalls(t *testing.T) {
	ctx := context.Background()
	t.Run("symlink", func(t *testing.T) {
		home := t.TempDir()
		real := filepath.Join(t.TempDir(), "elsewhere")
		if err := os.WriteFile(real, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		p := sbom.ToolPath(home, pinVersion)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(real, p); err != nil {
			t.Fatal(err)
		}
		fr := &fakeRunner{versions: map[string]string{p: pinVersion, real: pinVersion}}
		if _, err := sbom.Resolve(ctx, fr, home, sbom.ModeProcess, testPin()); !sbom.IsReason(err, sbom.ReasonToolMissing) || len(fr.cmds) != 0 {
			t.Fatalf("symlinked install: %v cmds=%v", err, fr.cmds)
		}
	})
	t.Run("writable by others", func(t *testing.T) {
		home := t.TempDir()
		p := installedSyft(t, home)
		if err := os.Chmod(p, 0o777); err != nil {
			t.Fatal(err)
		}
		fr := &fakeRunner{versions: map[string]string{p: pinVersion}}
		if _, err := sbom.Resolve(ctx, fr, home, sbom.ModeProcess, testPin()); !sbom.IsReason(err, sbom.ReasonToolMissing) || len(fr.cmds) != 0 {
			t.Fatalf("world-writable install: %v", err)
		}
	})
	t.Run("permission table", func(t *testing.T) {
		for _, mode := range []os.FileMode{0o644, 0o775, 0o757, 0o777, 0o4755} {
			home := t.TempDir()
			p := installedSyft(t, home)
			if err := os.Chmod(p, mode); err != nil {
				t.Fatal(err)
			}
			fr := &fakeRunner{versions: map[string]string{p: pinVersion}}
			if _, err := sbom.Resolve(ctx, fr, home, sbom.ModeProcess, testPin()); mode != 0o4755 && !sbom.IsReason(err, sbom.ReasonToolMissing) {
				t.Errorf("mode %o: %v", mode, err)
			}
			if len(fr.cmds) != 0 && mode != 0o4755 {
				t.Errorf("mode %o: the file was run", mode)
			}
		}
	})
	t.Run("group-writable directory", func(t *testing.T) {
		home := t.TempDir()
		p := installedSyft(t, home)
		for _, dir := range []string{filepath.Dir(p), filepath.Dir(filepath.Dir(p))} {
			if err := os.Chmod(dir, 0o775); err != nil {
				t.Fatal(err)
			}
			fr := &fakeRunner{versions: map[string]string{p: pinVersion}}
			if _, err := sbom.Resolve(ctx, fr, home, sbom.ModeProcess, testPin()); !sbom.IsReason(err, sbom.ReasonToolMissing) || len(fr.cmds) != 0 {
				t.Errorf("%s: %v", dir, err)
			}
			if err := os.Chmod(dir, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := sbom.Resolve(ctx, &fakeRunner{versions: map[string]string{p: pinVersion}}, home, sbom.ModeProcess, testPin()); err != nil {
			t.Fatalf("restored permissions: %v", err)
		}
	})
	t.Run("symlinked tools directory", func(t *testing.T) {
		home, real := t.TempDir(), t.TempDir()
		if err := os.MkdirAll(filepath.Join(real, "syft-"+pinVersion), 0o755); err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(real, "syft-"+pinVersion, "syft")
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(real, filepath.Join(home, "tools")); err != nil {
			t.Fatal(err)
		}
		fr := &fakeRunner{versions: map[string]string{sbom.ToolPath(home, pinVersion): pinVersion}}
		// Lstat of tools/syft-<v>/syft succeeds through the symlinked tools/ component, but
		// the tools/ entry itself is a symlink and is refused.
		if _, err := sbom.Resolve(ctx, fr, home, sbom.ModeProcess, testPin()); !sbom.IsReason(err, sbom.ReasonToolMissing) || len(fr.cmds) != 0 {
			t.Fatalf("symlinked tools/: %v cmds=%v", err, fr.cmds)
		}
	})
	t.Run("not a plain version", func(t *testing.T) {
		for _, v := range []string{"", "../x", "1/2", " 1", "-1"} {
			if _, err := sbom.Resolve(ctx, &fakeRunner{}, t.TempDir(), sbom.ModeProcess, sbom.Pin{Version: v}); !sbom.IsReason(err, sbom.ReasonToolMissing) {
				t.Errorf("version %q: %v", v, err)
			}
		}
	})
}

// A runner that fails for its own reasons is not "the tool is missing".
type brokenRunner struct{ fakeRunner }

func (b *brokenRunner) Run(context.Context, sbom.Cmd) (sbom.Result, error) {
	return sbom.Result{}, errors.New("symlink in job source: node_modules/.bin/x")
}

func TestProbeSeparatesRunnerFailuresFromAMissingTool(t *testing.T) {
	home := t.TempDir()
	installedSyft(t, home)
	_, err := sbom.Resolve(context.Background(), &brokenRunner{}, home, sbom.ModeProcess, testPin())
	if !sbom.IsReason(err, sbom.ReasonInvalid) || !strings.Contains(err.Error(), "symlink in job source") {
		t.Fatalf("runner failure: %v", err)
	}
	for _, exit := range []int{126, 127} {
		fr := &exitRunner{exit: exit}
		if _, err := sbom.Probe(context.Background(), fr, "/x/syft"); !sbom.IsReason(err, sbom.ReasonToolMissing) {
			t.Errorf("exit %d: %v", exit, err)
		}
	}
	if _, err := sbom.Probe(context.Background(), &exitRunner{exit: 2}, "/x/syft"); !sbom.IsReason(err, sbom.ReasonToolVersion) {
		t.Errorf("exit 2: %v", err)
	}
}

type exitRunner struct {
	fakeRunner
	exit int
}

func (e *exitRunner) Run(context.Context, sbom.Cmd) (sbom.Result, error) {
	return sbom.Result{Exit: e.exit}, nil
}

func TestProbeRunsInAnEmptyWorkspace(t *testing.T) {
	fr := &fakeRunner{versions: map[string]string{"/x/syft": pinVersion}}
	if _, err := sbom.Probe(context.Background(), fr, "/x/syft"); err != nil {
		t.Fatal(err)
	}
	if len(fr.cmds) != 1 || !fr.cmds[0].NoSource {
		t.Fatalf("the version probe must not copy the job source: %+v", fr.cmds)
	}
}

// D-1024: a file with the right owner, mode and self-reported version but other
// bytes than the pinned binary is refused, and never run.
func TestResolveRefusesAnInstalledBinaryThatIsNotTheDigestPinned(t *testing.T) {
	ctx := context.Background()
	t.Run("swapped file", func(t *testing.T) {
		home := t.TempDir()
		p := installedSyft(t, home)
		if err := os.WriteFile(p, []byte("#!/bin/sh\necho Version: "+pinVersion+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		fr := &fakeRunner{versions: map[string]string{p: pinVersion}}
		_, err := sbom.Resolve(ctx, fr, home, sbom.ModeProcess, testPin())
		if !sbom.IsReason(err, sbom.ReasonToolChecksum) || !strings.Contains(err.Error(), sbom.InstallCommand) || len(fr.cmds) != 0 {
			t.Fatalf("got %v cmds=%v", err, fr.cmds)
		}
	})
	t.Run("pin without a binary digest", func(t *testing.T) {
		home := t.TempDir()
		p := installedSyft(t, home)
		fr := &fakeRunner{versions: map[string]string{p: pinVersion}}
		_, err := sbom.Resolve(ctx, fr, home, sbom.ModeProcess, sbom.Pin{Version: pinVersion})
		if !sbom.IsReason(err, sbom.ReasonToolChecksum) || len(fr.cmds) != 0 {
			t.Fatalf("got %v cmds=%v", err, fr.cmds)
		}
	})
}
