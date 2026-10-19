package toolchain

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/doctor"
)

func probeFake(outputs map[string]string) doctor.CaptureFunc {
	return func(_ context.Context, argv ...string) (string, int, error) {
		if out, ok := outputs[argv[0]]; ok {
			return out, 0, nil
		}
		return "", 127, nil
	}
}

func checksByID(cs []doctor.Check, id string) []doctor.Check {
	var out []doctor.Check
	for _, c := range cs {
		if c.ID == id {
			out = append(out, c)
		}
	}
	return out
}

func TestSectionIsRegistered(t *testing.T) {
	for _, s := range doctor.Registered() {
		if s.Name == SectionName && s.Selector == doctor.SelectAlways {
			return
		}
	}
	t.Fatalf("section %s not registered", SectionName)
}

func TestDoctorSectionListsDriftImageAndVersions(t *testing.T) {
	root := "../../../.."
	env := doctor.Env{Root: root, Capture: probeFake(map[string]string{
		"go":       "go version go1.27.2 linux/arm64",
		"node":     "v24.21.0",
		"gitleaks": "8.30.1",
	})}
	checks := runSection(context.Background(), env)
	if c := checksByID(checks, "CI-DI01"); len(c) != 1 || c[0].Status != doctor.Pass || !strings.Contains(c[0].Message, "matches") {
		t.Fatalf("drift check %+v", c)
	}
	if c := checksByID(checks, "CI-DI02"); len(c) != 1 || c[0].Status != doctor.Pass || !strings.Contains(c[0].Message, "pinned image docker.io/nself/ci-toolchain@sha256:") {
		t.Fatalf("image check %+v", c)
	}
	warns := checksByID(checks, "CI-DI03")
	if len(warns) != 1 || warns[0].Status != doctor.Warn || warns[0].Subject != "node" || !strings.Contains(warns[0].Message, "24.21.0") {
		t.Fatalf("version checks %+v", warns)
	}
}

func TestDoctorAllMatchingIsOnePass(t *testing.T) {
	env := doctor.Env{Root: t.TempDir(), Capture: probeFake(map[string]string{"go": "go version go1.27.2 darwin/arm64", "govulncheck": "Go: go1.27.2\nScanner: govulncheck@v1.9.0\n"})}
	checks := runSection(context.Background(), env)
	got := checksByID(checks, "CI-DI03")
	if len(got) != 1 || got[0].Status != doctor.Pass || !strings.Contains(got[0].Message, "2 installed") {
		t.Fatalf("checks %+v", got)
	}
	if c := checksByID(checks, "CI-DI01"); !strings.Contains(c[0].Message, "no toolchain pins") {
		t.Fatalf("no-pins message %q", c[0].Message)
	}
}

func TestDoctorFlagsDriftedVersionsFile(t *testing.T) {
	dir := t.TempDir()
	tc := filepath.Join(dir, "free", "ci", "toolchain")
	if err := os.CopyFS(tc, os.DirFS(toolchainDir)); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(tc, "versions.yaml"))
	if err := os.WriteFile(filepath.Join(tc, "versions.yaml"), []byte(strings.Replace(string(b), "8.30.1", "8.30.2", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	c := checksByID(runSection(context.Background(), doctor.Env{Root: dir}), "CI-DI01")
	if len(c) != 1 || c[0].Status != doctor.Fail || !strings.Contains(c[0].Remediation, "go generate") {
		t.Fatalf("drift not reported: %+v", c)
	}
}

// TestDoctorIgnoresForeignPinsAndMissingVersionsFile: a repository with its own toolchain/pins is not
// judged, and a checkout of this plugin without versions.yaml has nothing on disk to compare.
func TestDoctorIgnoresForeignPinsAndMissingVersionsFile(t *testing.T) {
	foreign := t.TempDir()
	if err := os.MkdirAll(filepath.Join(foreign, "toolchain", "pins"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(foreign, "toolchain", "pins", "x.yaml"), []byte("garbage: ["), 0o644); err != nil {
		t.Fatal(err)
	}
	if c := checksByID(runSection(context.Background(), doctor.Env{Root: foreign}), "CI-DI01"); c[0].Status != doctor.Pass {
		t.Fatalf("foreign pins judged: %+v", c)
	}
	own := t.TempDir()
	if err := os.CopyFS(filepath.Join(own, "free", "ci", "toolchain"), os.DirFS(toolchainDir)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(own, "free", "ci", "toolchain", "versions.yaml")); err != nil {
		t.Fatal(err)
	}
	if c := checksByID(runSection(context.Background(), doctor.Env{Root: own}), "CI-DI01"); c[0].Status != doctor.Pass {
		t.Fatalf("missing versions.yaml failed the check: %+v", c)
	}
}

// TestDoctorFlagsStaleEmbeddedTable: pins newer than the table inside the binary fail the check.
func TestDoctorFlagsStaleEmbeddedTable(t *testing.T) {
	dir := t.TempDir()
	tc := filepath.Join(dir, "free", "ci", "toolchain")
	if err := os.CopyFS(tc, os.DirFS(toolchainDir)); err != nil {
		t.Fatal(err)
	}
	pin := filepath.Join(tc, "pins", "gitleaks.yaml")
	b, _ := os.ReadFile(pin)
	if err := os.WriteFile(pin, []byte(strings.Replace(string(b), "8.30.1", "8.30.2", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	c := checksByID(runSection(context.Background(), doctor.Env{Root: dir}), "CI-DI01")
	if len(c) != 1 || c[0].Status != doctor.Fail || !strings.Contains(c[0].Message, "embedded") {
		t.Fatalf("stale embedded table not reported: %+v", c)
	}
}

// TestDoctorIgnoresAnUnversionedBuild: the pinned image's govulncheck prints v0.0.0, which is not a drift.
func TestDoctorIgnoresAnUnversionedBuild(t *testing.T) {
	env := doctor.Env{Root: t.TempDir(), Capture: probeFake(map[string]string{"govulncheck": "Go: go1.27.2\nScanner: govulncheck@v0.0.0\n", "go": "go version go1.27.2 linux/arm64"})}
	got := checksByID(runSection(context.Background(), env), "CI-DI03")
	if len(got) != 1 || got[0].Status != doctor.Pass || !strings.Contains(got[0].Message, "1 installed") {
		t.Fatalf("checks %+v", got)
	}
}

func TestInstalledVersionParsing(t *testing.T) {
	cases := []struct{ name, out, want string }{
		{"go", "go version go1.27.2 linux/arm64", "1.27.2"},
		{"node", "v24.22.0\n", "24.22.0"},
		{"rust", "cargo 1.99.1 (abc 2026-10-13)", "1.99.1"},
		{"govulncheck", "Go: go1.27.2\nScanner: govulncheck@v1.9.0\nDB: x", "1.9.0"},
		{"hadolint", "Haskell Dockerfile Linter 2.15.1", "2.15.1"},
		{"trivy", "Version: 0.76.0", "0.76.0"},
		{"govulncheck", "Go: go1.27.2\nScanner: govulncheck@v0.0.0\nDB: https://vuln.go.dev", "0.0.0"},
		{"none", "no digits here", ""},
	}
	for _, c := range cases {
		if got := installedVersion(c.name, c.out); got != c.want {
			t.Errorf("%s: %q -> %q, want %q", c.name, c.out, got, c.want)
		}
	}
}
