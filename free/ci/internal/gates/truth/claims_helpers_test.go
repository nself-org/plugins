package truth

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// realClaims is the shipped claims.yaml, the file every fixture below runs against.
func realClaims(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", ".github", "truth", "claims.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Replace(string(b), "generated_regions:\n", "generated_regions:\n  facts: fixture generated fact region\n  facts:top-level: fixture colon region\n  x: fixture generated text region\n", 1)
}

// claimFindings scans files with the shipped claims.yaml (as .github/truth/claims.yaml)
// and returns "rule path:line" for every finding, sorted.
func claimFindings(t *testing.T, files map[string]string) []string {
	t.Helper()
	files[".github/truth/claims.yaml"] = realClaims(t)
	return findingsOf(t, files, ".github/truth/claims.yaml")
}

// findingsOf scans files with the rule files and returns "rule path:line" per finding.
func findingsOf(t *testing.T, files map[string]string, ruleFiles ...string) []string {
	t.Helper()
	dir := writeTree(t, files)
	rs, err := Load(dir, ruleFiles)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Scan(dir, rs, rs.Files)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range res.Findings {
		got = append(got, f.RuleID+" "+f.Path+":"+strconv.Itoa(f.Line))
	}
	sort.Strings(got)
	return got
}

func wantFindings(t *testing.T, got []string, want ...string) {
	t.Helper()
	sort.Strings(want)
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("findings\n got  %q\n want %q", got, want)
	}
}

// the retired names are built at run time: this file is itself scanned by the org rules.
func proPlugins() string { return "Pro " + "plugins" }
func oldSlug() string    { return "nself-org/" + "plugins-pro" }
