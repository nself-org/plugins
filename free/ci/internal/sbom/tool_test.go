package sbom_test

import (
	"context"
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
			if !ok || len(s.SHA256) != 64 || !strings.HasPrefix(s.URL, "https://") {
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
	script := "#!/bin/sh\ntouch " + marker + "\necho 'Version: 1.54.1'\n"
	if err := os.WriteFile(filepath.Join(dir, "syft"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	_, err := sbom.Resolve(context.Background(), execRunner(t, t.TempDir()), t.TempDir(), sbom.ModeProcess, testPin())
	if !sbom.IsReason(err, sbom.ReasonToolVersion) {
		t.Fatalf("got %v", err)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatal("the planted PATH syft was executed")
	}
}
