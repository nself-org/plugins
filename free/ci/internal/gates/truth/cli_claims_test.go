package truth

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"
)

// TestMainPrintsOneLinePerFinding: human mode prints `truth: <rule-id> <file>:<line>: <match>`.
func TestMainPrintsOneLinePerFinding(t *testing.T) {
	dir := writeTree(t, map[string]string{".github/truth/claims.yaml": realClaims(t), "README.md": "ok\nWe ship 129 free plugins.\n", "docs/a.md": "Buy " + proPlugins() + "\n"})
	code, out, _ := runMain("", "check", "--root", dir, ".github/truth/claims.yaml")
	if code != 1 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	var lines []string
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "truth: ") {
			lines = append(lines, l)
		}
	}
	want := []string{"truth: claims.counts README.md:2: 129 free plugins", "truth: vocab.pro-plugin docs/a.md:1: " + proPlugins()}
	sort.Strings(lines)
	sort.Strings(want)
	if strings.Join(lines, "|") != strings.Join(want, "|") {
		t.Errorf("lines %q, want %q", lines, want)
	}
	for _, id := range []string{"FAIL truth.numeric-claim (1 finding(s))", "PASS truth.version-claim", "FAIL truth.retired-term (1 finding(s))"} {
		if !strings.Contains(out, id) {
			t.Errorf("missing %q in\n%s", id, out)
		}
	}
	if code, out, _ := runMain("", "check", "--root", dir, "--kind", "version-claim", ".github/truth/claims.yaml"); code != 0 || strings.Count(out, "truth.") != 1 {
		t.Errorf("one claim kind: %d %q", code, out)
	}
}

// TestMainJSON: --json prints one document with data.result.
func TestMainJSON(t *testing.T) {
	dir := writeTree(t, map[string]string{".github/truth/claims.yaml": realClaims(t), "README.md": "We ship 129 free plugins.\n"})
	code, out, _ := runMain("", "check", "--root", dir, "--json", ".github/truth/claims.yaml")
	var doc struct {
		Data struct {
			Result   string
			Findings []struct {
				Rule, File, Match string
				Line              int
			}
		}
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil || code != 1 || doc.Data.Result != "fail" || len(doc.Data.Findings) != 1 || doc.Data.Findings[0].Rule != "claims.counts" || doc.Data.Findings[0].Line != 1 {
		t.Fatalf("%d %v\n%s", code, err, out)
	}
	clean := writeTree(t, map[string]string{".github/truth/claims.yaml": realClaims(t), "README.md": "fine\n"})
	if code, out, _ := runMain("", "check", "--root", clean, "--json", ".github/truth/claims.yaml"); code != 0 || !strings.Contains(out, `"result":"pass"`) || !strings.Contains(out, `"findings":[]`) {
		t.Errorf("clean: %d %s", code, out)
	}
	bad := writeTree(t, map[string]string{"r.yaml": "version: 1\nrules:\n  - {id: a, kind: nope, message: m}\n"})
	if code, out, _ := runMain("", "check", "--root", bad, "--json", "r.yaml"); code != 2 || !strings.Contains(out, `"result":"error"`) {
		t.Errorf("error: %d %s", code, out)
	}
}

// TestBaseKindsUnchanged: rule files with no claim rule still report exactly three checks.
func TestBaseKindsUnchanged(t *testing.T) {
	dir := writeTree(t, map[string]string{"rules.yaml": fixtureRules(), "ok.md": "x\n"})
	if n := len(Run(dir, []string{"rules.yaml"}, "")); n != 3 {
		t.Errorf("checks = %d", n)
	}
}

// TestKindFiltersFindings: --kind with --json lists only that kind's findings; a rule-file error is shown for a claim kind.
func TestKindFiltersFindings(t *testing.T) {
	dir := writeTree(t, map[string]string{".github/truth/claims.yaml": realClaims(t), "README.md": "We ship 129 free plugins.\nBuy " + proPlugins() + "\n"})
	code, out, _ := runMain("", "check", "--root", dir, "--kind", "version-claim", "--json", ".github/truth/claims.yaml")
	if code != 0 || !strings.Contains(out, `"findings":[]`) {
		t.Errorf("version-claim only: %d %s", code, out)
	}
	bad := writeTree(t, map[string]string{"r.yaml": "version: 1\nrules:\n  - {id: a, kind: nope, message: m}\n"})
	if code, out, _ := runMain("", "check", "--root", bad, "--kind", "numeric-claim", "r.yaml"); code != 2 || !strings.Contains(out, "E644 r.yaml: rules[0].kind") {
		t.Errorf("load error under a claim kind: %d %s", code, out)
	}
}
