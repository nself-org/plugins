package bench

// Purpose: build the binary under test and name the revision measured.
// Inputs:  the free/ci module root and an output path.
// Outputs: the binary path; the git HEAD of the tree, with -dirty appended when
// tracked files changed.
// Constraints: CGO_ENABLED=0 and -mod=vendor, as every build of this module.

import (
	"context"
	"fmt"
	"strings"
)

// BuildBinary builds ./cmd of the module at root into out.
func BuildBinary(ctx context.Context, root, out string) (string, error) {
	env := append(baseEnv(), "CGO_ENABLED=0")
	if _, err := must(ctx, []string{"go", "build", "-mod=vendor", "-o", out, "./cmd"}, root, env); err != nil {
		return "", fmt.Errorf("build nself-ci: %w", err)
	}
	return out, nil
}

// TreeSHA is the git HEAD of dir, "-dirty" when tracked files are modified, or
// "" when dir is not a checkout.
func TreeSHA(ctx context.Context, dir string) string {
	r, err := must(ctx, []string{"git", "rev-parse", "HEAD"}, dir, baseEnv())
	if err != nil {
		return ""
	}
	sha := strings.TrimSpace(r.Tail)
	if st, err := must(ctx, []string{"git", "status", "--porcelain", "--untracked-files=no"}, dir, baseEnv()); err == nil && st.Tail != "" {
		sha += "-dirty"
	}
	return sha
}

// Format renders a result as aligned text lines (stderr summary).
func Format(r Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s/%s runs=%d sha=%s\n", r.GOOS, r.GOARCH, r.Runs, r.SHA)
	for _, m := range r.Metrics {
		fmt.Fprintf(&b, "%-24s p50 %8.1f %s  p95 %8.1f %s  max %8.1f %s  n=%d\n", m.Name, m.P50, m.Unit, m.P95, m.Unit, m.Max, m.Unit, m.N)
	}
	return strings.TrimRight(b.String(), "\n")
}
