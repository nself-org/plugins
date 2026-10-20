//go:build scenarios

package scenarios

// Purpose: the acceptance matrix check. ACCEPTANCE.md maps every test of this suite to the Epic
// SCHED acceptance item it proves (2 scenario 4, 3 scenario 6, 4 scenario 10, 5 economics, 6 burst);
// this test fails when a suite test is missing from the matrix, a matrix row names a test that does
// not exist, a row carries an item outside 2-6, or an item has no test.
// Inputs:  the *_test.go files of this directory and ACCEPTANCE.md.
// Outputs: assertions only.
// Constraints: reads files in the package directory only; build tags are ignored on purpose, so the
//          planted tests (tag `planted`) must be in the matrix too.

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var (
	testFunc  = regexp.MustCompile(`(?m)^func (Test[A-Za-z0-9_]+)\(`)
	matrixRow = regexp.MustCompile("(?m)^\\| `(Test[A-Za-z0-9_]+)` \\| ([0-9, -]+) \\|")
)

func suiteTests(t *testing.T) map[string]bool {
	t.Helper()
	files, err := filepath.Glob("*_test.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("no test files found: %v", err)
	}
	out := map[string]bool{}
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range testFunc.FindAllStringSubmatch(string(src), -1) {
			out[m[1]] = true
		}
	}
	return out
}

func TestAcceptanceMatrixMapsEveryTest(t *testing.T) {
	raw, err := os.ReadFile("ACCEPTANCE.md")
	if err != nil {
		t.Fatalf("the acceptance matrix is missing: %v", err)
	}
	tests := suiteTests(t)
	rows := map[string][]string{}
	covered := map[string]bool{}
	for _, m := range matrixRow.FindAllStringSubmatch(string(raw), -1) {
		items := strings.FieldsFunc(m[2], func(r rune) bool { return r == ',' || r == ' ' })
		rows[m[1]] = items
		for _, it := range items {
			switch it {
			case "-": // a test that proves no acceptance item itself (this one): it never counts as coverage
			case "2", "3", "4", "5", "6":
				covered[it] = true
			default:
				t.Errorf("%s maps to %q: only Epic acceptance 2-6 belong to this suite", m[1], it)
			}
		}
	}
	var missing, stale []string
	for name := range tests {
		if _, ok := rows[name]; !ok {
			missing = append(missing, name)
		}
	}
	for name := range rows {
		if !tests[name] {
			stale = append(stale, name)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	if len(missing) > 0 {
		t.Errorf("tests missing from ACCEPTANCE.md: %v", missing)
	}
	if len(stale) > 0 {
		t.Errorf("ACCEPTANCE.md rows with no test: %v", stale)
	}
	for _, item := range []string{"2", "3", "4", "5", "6"} {
		if !covered[item] {
			t.Errorf("Epic acceptance %s has no test in the matrix", item)
		}
	}
}
