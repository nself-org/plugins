package gates

import (
	"path/filepath"
	"strings"
)

// dropVendored removes gofmt output lines (file paths or parse errors) that
// point inside a vendor/ directory at any depth. `go mod vendor` output is
// third-party code the repo does not format, so it must not fail go.fmt;
// cli scripts/ci/gofmt-check.sh skips it the same way (D-0286).
func dropVendored(out string) string {
	if out == "" {
		return out
	}
	var kept []string
	for _, line := range strings.Split(out, "\n") {
		p := filepath.ToSlash(strings.TrimSpace(line))
		if strings.HasPrefix(p, "vendor/") || strings.Contains(p, "/vendor/") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}
