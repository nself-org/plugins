package main

import "fmt"

// formatBytes returns a human-readable size string.
//
// Copied from cli/cmd/commands/migrate_from_bash.go (CLI-R11): the original
// was a package-wide utility shared by model.go/ollama.go and unrelated
// migrate commands that stayed in core, so it could not simply move — it is
// a tiny, stable, self-contained function, not a real shared dependency.
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
