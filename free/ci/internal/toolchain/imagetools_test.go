package toolchain

import (
	"slices"
	"testing"
)

func TestImageToolsHasNoSwift(t *testing.T) {
	tools, err := ImageTools()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"go", "node", "npm", "git", "cargo", "uv", "ruff", "gitleaks"} {
		if !slices.Contains(tools, want) {
			t.Errorf("image tools lack %s: %v", want, tools)
		}
	}
	for _, not := range []string{"swift", "xcodebuild", "xcode", "docker"} {
		if slices.Contains(tools, not) {
			t.Errorf("image tools list %s, which the image does not ship: %v", not, tools)
		}
	}
}
