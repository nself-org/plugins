package truth

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runMain(in string, args ...string) (code int, out, errw string) {
	var o, e bytes.Buffer
	code = Main(args, strings.NewReader(in), &o, &e)
	return code, o.String(), e.String()
}

func TestMainCheckExitCodesAndKinds(t *testing.T) {
	clean := writeTree(t, map[string]string{"rules.yaml": fixtureRules(), "a.md": "fine\n"})
	dirty := writeTree(t, map[string]string{"rules.yaml": fixtureRules(), "a.md": "Old Name\n"})
	if code, out, _ := runMain("", "check", "--root", clean, "rules.yaml"); code != 0 || strings.Count(out, "PASS truth.") != 3 {
		t.Errorf("clean: %d %q", code, out)
	}
	code, out, _ := runMain("", "check", "--root", dirty, "rules.yaml")
	if code != 1 || !strings.Contains(out, "FAIL truth.retired-term (1 finding(s))") || !strings.Contains(out, "truth: retired-oldname a.md:1: Old Name") {
		t.Errorf("dirty: %d %q", code, out)
	}
	if code, out, _ := runMain("", "check", "--root", dirty, "--kind", "dead-host", "rules.yaml"); code != 0 || strings.Count(out, "truth.") != 1 {
		t.Errorf("one kind only: %d %q", code, out)
	}
	if code, out, _ := runMain("", "check", "--root", dirty, "--kind", "retired-term", "rules.yaml"); code != 1 || strings.Count(out, "truth.") != 1 {
		t.Errorf("one failing kind: %d %q", code, out)
	}
	if code, out, _ := runMain("", "check", "--root", dirty, "--kind", "rules", "rules.yaml"); code != 0 || !strings.Contains(out, "PASS truth.rules (4 rule(s) in 1 file(s))") {
		t.Errorf("rules only: %d %q", code, out)
	}
	bad := writeTree(t, map[string]string{"r.yaml": "version: 1\nrules:\n  - {id: a, kind: nope, message: m}\n"})
	if code, out, _ := runMain("", "check", "--root", bad, "--kind", "rules", "r.yaml"); code != 2 || !strings.Contains(out, "E644 r.yaml: rules[0].kind") {
		t.Errorf("bad rules: %d %q", code, out)
	}
	if code, out, _ := runMain("", "check", "--root", bad, "r.yaml"); code != 2 || !strings.Contains(out, "E644 r.yaml: rules[0].kind") {
		t.Errorf("bad rules, full check: %d %q", code, out)
	}
	if code, _, _ := runMain("", "check", "--root", clean, "--kind", "other", "rules.yaml"); code != 2 {
		t.Errorf("unknown kind: %d", code)
	}
	for _, args := range [][]string{{}, {"nope"}, {"check"}, {"sweep"}} {
		if code, _, _ := runMain("", args...); code != 2 {
			t.Errorf("%v: %d", args, code)
		}
	}
}

func TestMainSweepPrintsOneEvidenceLine(t *testing.T) {
	clean := writeTree(t, map[string]string{"a.md": "fine\n"})
	dirty := writeTree(t, map[string]string{"a.md": "Old Name\n"})
	rules := writeTree(t, map[string]string{"rules.yaml": fixtureRules()})
	code, out, errw := runMain("", "sweep", "--rules-root", rules, "--revision", testRevision, "rules.yaml", "--", "good="+clean, "bad="+dirty, "gone=!clone failed")
	if code != 2 {
		t.Errorf("a clone failure must give exit 2, got %d", code)
	}
	if strings.Count(strings.TrimSpace(out), "\n") != 0 || !strings.Contains(out, `"ci.evidence/v1"`) {
		t.Errorf("stdout must be exactly one document line: %q", out)
	}
	var doc struct {
		Result string
		Checks []struct{ ID, Result string }
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil || len(doc.Checks) != 3 {
		t.Fatalf("%v %+v", err, doc)
	}
	if !strings.Contains(errw, "FAIL truth-sweep.bad") || !strings.Contains(errw, "ERROR truth-sweep.gone") || !strings.Contains(errw, "PASS truth-sweep.good") {
		t.Errorf("stderr: %q", errw)
	}
	if code, _, _ := runMain("", "sweep", "--rules-root", rules, "--revision", testRevision, "rules.yaml", "--", "good="+clean); code != 0 {
		t.Errorf("all clean must exit 0, got %d", code)
	}
	if code, _, _ := runMain("", "sweep", "--rules-root", rules, "--revision", testRevision, "rules.yaml", "--", "bad="+dirty); code != 1 {
		t.Errorf("a broken rule must exit 1, got %d", code)
	}
}

func TestMainHashReadsStdin(t *testing.T) {
	code, out, _ := runMain(plantedHost()+"\n\n"+strings.ToUpper(plantedName())+"\n", "hash")
	want := HashLiteral(plantedHost()) + "\n" + HashLiteral(plantedName()) + "\n"
	if code != 0 || out != want {
		t.Errorf("%d %q", code, out)
	}
	if code, _, _ := runMain("", "hash"); code != 2 {
		t.Errorf("empty stdin: %d", code)
	}
}

// buildRunner compiles run.go (build-ignored, so `go vet ./...` skips it) and
// returns the binary: it proves the file still builds against the package.
func buildRunner(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not on PATH")
	}
	bin := filepath.Join(t.TempDir(), "truth-runner")
	cmd := exec.Command("go", "build", "-mod=vendor", "-o", bin, "./internal/gates/truth/run.go")
	cmd.Dir = filepath.Join("..", "..", "..")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run.go does not build: %v\n%s", err, out)
	}
	return bin
}

func TestRunnerBuildsAndChecks(t *testing.T) {
	bin := buildRunner(t)
	dir := writeTree(t, map[string]string{"rules.yaml": fixtureRules(), "a.md": "SEKRET_MARKER\n"})
	out, err := exec.Command(bin, "truth", "--root", dir, "rules.yaml").CombinedOutput()
	var ee *exec.ExitError
	if !asExit(err, &ee) || ee.ExitCode() != 1 || !strings.Contains(string(out), "FAIL truth.banned-literal") {
		t.Errorf("%v %s", err, out)
	}
}

func asExit(err error, target **exec.ExitError) bool {
	e, ok := err.(*exec.ExitError)
	if ok {
		*target = e
	}
	return ok
}

func TestSweepRefusesABadRevision(t *testing.T) {
	rules := writeTree(t, map[string]string{"rules.yaml": fixtureRules()})
	for _, rev := range []string{"", "abc", strings.Repeat("G", 40)} {
		if code, _, _ := runMain("", "sweep", "--rules-root", rules, "--revision", rev, "rules.yaml", "--", "x="+rules); code != 2 {
			t.Errorf("revision %q: exit %d", rev, code)
		}
	}
}
