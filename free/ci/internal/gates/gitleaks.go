package gates

import (
	"os"
	"path/filepath"
)

// gitleaksCommand preserves legacy config precedence and filesystem fallback.
func gitleaksCommand(root string) []string {
	argv := []string{"gitleaks", "detect", "--source", ".", "--exit-code", "1"}
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		argv = append(argv, "--no-git")
	}
	for _, name := range []string{".github/gitleaks.toml", ".gitleaks.toml", "gitleaks.toml"} {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			argv = append(argv, "--config", name)
			break
		}
	}
	return argv
}
