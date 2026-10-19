package gates

import "testing"

// D-1026: the missing-tool classification is shared by Runner.Run and the run
// loop's outcome hook.
func TestUnavailableClassifiesMissingTools(t *testing.T) {
	for _, tc := range []struct {
		id, out, tool string
		exit          int
		want          bool
	}{
		{"python-test", "error: Failed to spawn: `pytest`", "pytest", 2, true},
		{"python-test", "/usr/bin/python: No module named pytest", "pytest", 1, true},
		{"python-types", "sh: mypy: command not found", "mypy", 127, true},
		{"python-types", "error: Failed to spawn: `pytest`", "", 2, false}, // the other check's tool
		{"python-test", "1 failed, 2 passed", "", 1, false},
		{"python-test", "No module named pytest", "", 0, false}, // exit 0 is never reclassified
		{"go-test", "No module named pytest", "", 1, false},
	} {
		tool, fix, ok := Unavailable(tc.id, tc.exit, tc.out)
		if ok != tc.want || tool != tc.tool || (ok && fix == "") {
			t.Errorf("Unavailable(%q, %d, %q) = %q %q %v", tc.id, tc.exit, tc.out, tool, fix, ok)
		}
	}
}
