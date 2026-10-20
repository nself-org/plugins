//go:build security

package security

// Purpose: keep matrix.yaml honest (P7-TRUST-17 acceptance). Every directive item, scenario and review
// addition has a case that exists; every threat row of the threat model has a case, a module proof that
// exists, or a named owner; every test of this suite is named in the matrix; the planted regressions
// cover the eight categories the Ticket lists.
// Inputs:  matrix.yaml, the Go sources of this package and of the module, testdata/regressions, and (when
//          $HQ names the hq checkout) architecture/ci-threat-model.md.
// Outputs: pass or fail.
// Constraints: static: no test is run from here; "also" proofs are located by parsing the module's test
//          files, so a renamed proof breaks this test instead of rotting in the YAML.

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type caseRef struct {
	Test string `yaml:"test"`
}

type item struct {
	ID    string    `yaml:"id"`
	Item  string    `yaml:"item"`
	Cases []caseRef `yaml:"cases"`
}

type proofRef struct {
	Pkg   string `yaml:"pkg"`
	Proof string `yaml:"proof"`
}

type threat struct {
	ID    string     `yaml:"id"`
	Cases []caseRef  `yaml:"cases"`
	Also  []proofRef `yaml:"also"`
	Owner string     `yaml:"owner"`
}

type matrixDoc struct {
	Version         int      `yaml:"version"`
	Scenarios       []item   `yaml:"scenarios"`
	DirectiveItems  []item   `yaml:"directive_items"`
	ReviewAdditions []item   `yaml:"review_additions"`
	Threats         []threat `yaml:"threats"`
}

func loadMatrix(t *testing.T) matrixDoc {
	t.Helper()
	raw, err := os.ReadFile("matrix.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var m matrixDoc
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err = dec.Decode(&m); err != nil {
		t.Fatalf("matrix.yaml: %v", err)
	}
	return m
}

// testFuncs lists the top-level Test functions declared in the *_test.go files of dir.
func testFuncs(t *testing.T, dir string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	files, err := filepath.Glob(filepath.Join(dir, "*_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, f := range files {
		file, err := parser.ParseFile(fset, f, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		for _, d := range file.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv == nil && strings.HasPrefix(fn.Name.Name, "Test") {
				out[fn.Name.Name] = true
			}
		}
	}
	return out
}

var (
	wantScenarios = []string{"scenario-7", "scenario-8"}
	wantDirective = []string{"untrusted-pr-no-lan-no-secrets", "private-only-never-external", "scoped-runner-credentials", "revoked-runner-no-jobs", "log-redaction",
		"no-workspace-leakage", "artifacts-authorized", "cache-respects-trust", "no-isolation-downgrade", "forged-stale-evidence", "authenticated-webhooks",
		"authorized-control-plane", "prod-infra-ineligible"}
	wantReview = []string{"dispatcher-invariants", "planted-pr-policy", "payload-to-class", "approval-voided-by-push", "revoked-node-no-job", "node-cert-admin-api",
		"untrusted-exclusion-onlink", "telemetry-zero", "publication-profile", "cotenancy", "uid-separation", "lease-holder-signature", "approval-checkout-binding"}
	ownerPattern  = regexp.MustCompile(`^P7-[A-Z]+-\d+$`)
	threatPattern = regexp.MustCompile(`^TM-\d{2}$`)
)

func TestMatrixIsComplete(t *testing.T) {
	m := loadMatrix(t)
	if m.Version != 1 {
		t.Fatalf("matrix version %d", m.Version)
	}
	here := testFuncs(t, ".")
	used := map[string]bool{}
	checkCases := func(where string, cases []caseRef) {
		for _, c := range cases {
			if !here[c.Test] {
				t.Errorf("%s names %q, which is not a test of this package", where, c.Test)
			}
			used[c.Test] = true
		}
	}
	section := func(name string, got []item, want []string) {
		seen := map[string]bool{}
		for _, it := range got {
			if seen[it.ID] {
				t.Errorf("%s: id %s appears twice", name, it.ID)
			}
			seen[it.ID] = true
			if it.Item == "" || len(it.Cases) == 0 {
				t.Errorf("%s %s has no description or no case", name, it.ID)
			}
			checkCases(name+" "+it.ID, it.Cases)
		}
		for _, w := range want {
			if !seen[w] {
				t.Errorf("%s: required item %s is missing from the matrix", name, w)
			}
		}
		if len(got) != len(want) {
			t.Errorf("%s: %d items, %d required: update wantXxx in matrix_test.go with the matrix", name, len(got), len(want))
		}
	}
	section("scenarios", m.Scenarios, wantScenarios)
	section("directive_items", m.DirectiveItems, wantDirective)
	section("review_additions", m.ReviewAdditions, wantReview)

	moduleFuncs := map[string]map[string]bool{}
	ids := map[string]bool{}
	for _, th := range m.Threats {
		if !threatPattern.MatchString(th.ID) || ids[th.ID] {
			t.Errorf("threat id %q is malformed or repeated", th.ID)
		}
		ids[th.ID] = true
		checkCases(th.ID, th.Cases)
		for _, a := range th.Also {
			dir := filepath.Join("..", "..", filepath.FromSlash(a.Pkg))
			if moduleFuncs[dir] == nil {
				moduleFuncs[dir] = testFuncs(t, dir)
			}
			if !moduleFuncs[dir][a.Proof] {
				t.Errorf("%s: proof %s is not a test of %s", th.ID, a.Proof, a.Pkg)
			}
		}
		if th.Owner != "" && !ownerPattern.MatchString(th.Owner) {
			t.Errorf("%s: owner %q is not a Ticket id", th.ID, th.Owner)
		}
		if len(th.Cases) == 0 && len(th.Also) == 0 && th.Owner == "" {
			t.Errorf("%s has no case, no proof and no owner", th.ID)
		}
		if len(th.Cases) == 0 && len(th.Also) == 0 {
			t.Logf("%s is carried by its owner Ticket %s, not by this suite", th.ID, th.Owner)
		}
	}
	// Every test of the suite is named in the matrix (except the checks of the matrix itself).
	var orphan []string
	for name := range here {
		if !used[name] && name != "TestMain" && name != "TestMatrixIsComplete" && name != "TestRegressionPatchesCoverTheCategories" {
			orphan = append(orphan, name)
		}
	}
	sort.Strings(orphan)
	if len(orphan) > 0 {
		t.Errorf("tests that no matrix row names: %v", orphan)
	}
	// The matrix has exactly the threat rows listed in testdata/threat-rows.txt (a copy of the ids of
	// hq architecture/ci-threat-model.md; a row added there fails this until the matrix maps it).
	listed := map[string]bool{}
	rows, err := os.ReadFile(filepath.Join("testdata", "threat-rows.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range strings.Fields(string(rows)) {
		listed[id] = true
		if !ids[id] {
			t.Errorf("threat row %s (testdata/threat-rows.txt) has no matrix row", id)
		}
	}
	for id := range ids {
		if !listed[id] {
			t.Errorf("matrix row %s is not in testdata/threat-rows.txt", id)
		}
	}
	// With the hq checkout at hand, that list is exactly the threat model's rows.
	if hq := os.Getenv("HQ"); hq != "" {
		raw, err := os.ReadFile(filepath.Join(hq, "architecture", "ci-threat-model.md"))
		if err != nil {
			t.Fatalf("$HQ is set but the threat model cannot be read: %v", err)
		}
		model := map[string]bool{}
		for _, m := range regexp.MustCompile(`(?m)^\| (TM-\d{2}) \|`).FindAllStringSubmatch(string(raw), -1) {
			model[m[1]] = true
		}
		if len(model) == 0 {
			t.Fatal("no TM rows found in the threat model")
		}
		for id := range model {
			if !listed[id] {
				t.Errorf("threat model row %s is missing from testdata/threat-rows.txt", id)
			}
		}
		for id := range listed {
			if !model[id] {
				t.Errorf("testdata/threat-rows.txt row %s is not in the threat model", id)
			}
		}
	}
}

// TestRegressionPatchesCoverTheCategories: the planted regressions include one for each category the
// Ticket names, and there are at least eight patches.
func TestRegressionPatchesCoverTheCategories(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "regressions", "*.patch"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 8 {
		t.Fatalf("%d planted regressions, want at least eight", len(files))
	}
	categories := map[string]string{
		"secret to untrusted": "secret-to-untrusted", "LAN for untrusted": "lan-for-untrusted", "private job on hosted fake": "private-job-on-hosted",
		"stale evidence accepted": "stale-evidence-accepted", "deploy host selected": "deploy-host-selected", "policy read from the revision": "policy-read-from-revision",
		"approval surviving a push": "approval-survives-push", "CI data sent to telemetry": "ci-data-to-telemetry",
	}
	for label, fragment := range categories {
		found := false
		for _, f := range files {
			found = found || strings.Contains(filepath.Base(f), fragment)
		}
		if !found {
			t.Errorf("no planted regression for %q (a patch named *%s*)", label, fragment)
		}
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil || !(bytes.HasPrefix(raw, []byte("--- a/")) || bytes.HasPrefix(raw, []byte("diff --git a/"))) || !bytes.Contains(raw, []byte("\n+++ b/")) {
			t.Errorf("%s is not a unified diff with a/ b/ paths (%v)", f, err)
		}
	}
}
