package gates

import "testing"

func TestMissingResult(t *testing.T) {
	for _, tc := range []struct {
		check, output, tool string
		exit                int
	}{
		{"python-test", "No module named pytest", "pytest", 1},
		{"python-types", "Failed to spawn: `mypy`", "mypy", 1},
		{"xcode-test", "xcodebuild requires Xcode; Command Line Tools installed", "xcodebuild", 1},
		{"python-test", "No module named pytest", "", 0},
		{"python-test", "test failed", "", 1},
		{"custom", "No module named pytest", "", 1},
	} {
		t.Run(tc.check+"/"+tc.output, func(t *testing.T) {
			tool, fix := MissingResult(tc.check, tc.exit, tc.output)
			if tool != tc.tool || (tool != "" && fix == "") {
				t.Fatalf("tool = %q, remediation = %q", tool, fix)
			}
		})
	}
}
