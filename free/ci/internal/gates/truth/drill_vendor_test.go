package truth

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// vendorInto writes the vendored rules into repo and commits them.
func vendorInto(t *testing.T, repo string) {
	t.Helper()
	script, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "..", "scripts", "truth", "vendor-rules.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("bash", script, repo).CombinedOutput(); err != nil {
		t.Fatalf("vendor-rules.sh: %v\n%s", err, out)
	}
	gitIn(t, repo, "add", "-A")
	gitIn(t, repo, "commit", "-q", "-m", "vendor the rules")
}

// TestDrillFailsOnDriftedVendoredRules: a clone whose .github/truth/ differs
// from this checkout fails the sweep; an equal copy passes (TRUTH review F26).
func TestDrillFailsOnDriftedVendoredRules(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash is not on PATH")
	}
	base, list := fixtureOrg(t, map[string]string{"alpha": "fine\n", "beta": "fine\n"})
	vendorInto(t, filepath.Join(base, "alpha"))
	vendorInto(t, filepath.Join(base, "beta"))
	if code, out, errw := runDrill(t, base, list); code != 0 || strings.Contains(errw, "vendored-rules") || !strings.Contains(out, `"result":"pass"`) || strings.Count(strings.TrimSpace(out), "\n") != 0 {
		t.Fatalf("equal copies must pass: exit %d\n%s\n%s", code, errw, out)
	}

	f := filepath.Join(base, "beta", ".github", "truth", "claims.yaml")
	b, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f, append(b, []byte("# hand edit\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, filepath.Join(base, "beta"), "commit", "-q", "-am", "drift")
	code, out, errw := runDrill(t, base, list)
	if code != 2 || !strings.Contains(errw, "ERROR truth-sweep.beta.vendored-rules") || !strings.Contains(errw, "claims.yaml differs") || !strings.Contains(out, `"result":"error"`) {
		t.Fatalf("a drifted copy must fail the sweep: exit %d\n%s\n%s", code, errw, out)
	}
	if strings.Contains(errw, "alpha.vendored-rules") {
		t.Errorf("the equal copy must not be reported:\n%s", errw)
	}
}

// TestDrillSkipsTheSourceRepository: plugins holds the rules themselves, with no header.
func TestDrillSkipsTheSourceRepository(t *testing.T) {
	base, list := fixtureOrg(t, map[string]string{"plugins": "fine\n"})
	dir := filepath.Join(base, "plugins", ".github", "truth")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "org.yaml"), []byte("version: 1\nrules: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, filepath.Join(base, "plugins"), "add", "-A")
	gitIn(t, filepath.Join(base, "plugins"), "commit", "-q", "-m", "rules")
	if code, _, errw := runDrill(t, base, list); code != 0 || strings.Contains(errw, "vendored-rules") {
		t.Fatalf("exit %d\n%s", code, errw)
	}
}
