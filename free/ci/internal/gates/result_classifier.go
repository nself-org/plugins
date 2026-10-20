package gates

import "strings"

// MissingResult identifies a missing catalogue tool from a failed command's
// output, rather than mistaking an installed launcher for an installed tool.
func MissingResult(check string, exit int, output string) (tool, remediation string) {
	if exit == 0 {
		return "", ""
	}
	switch check {
	case "python-test":
		if pytestMissing(output) {
			return "pytest", "Add pytest to the project's dependencies"
		}
	case "python-types":
		for _, marker := range []string{"No module named mypy", "Failed to spawn: `mypy`", "mypy: command not found", "Command not found: mypy"} {
			if strings.Contains(output, marker) {
				return "mypy", "Add mypy to the project's dependencies"
			}
		}
	case "xcode-test":
		if strings.Contains(output, "requires Xcode") && strings.Contains(strings.ToLower(output), "command line tools") {
			return "xcodebuild", "Run on a Mac with Xcode installed and selected with xcode-select"
		}
	}
	return "", ""
}
