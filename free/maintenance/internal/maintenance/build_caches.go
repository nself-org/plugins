package maintenance

import (
	"fmt"
	"os/exec"
	"strings"
)

// BuildCacheResult reports what CleanBuildCaches did, per tool.
type BuildCacheResult struct {
	Before  DiskUsage
	After   DiskUsage
	Ran     []string // tools whose cache was cleared
	Skipped []string // tools not installed on this host
	Errors  []error
}

// buildCacheCleaner is one tool's cache-clearing command. Each uses the tool's
// OWN cleanup subcommand rather than deleting a guessed path: the tools know
// where their caches live and which parts are safe to drop, and a wrong path
// in an rm would be a very expensive mistake to make on a shared host.
type buildCacheCleaner struct {
	tool string
	args []string
}

var buildCacheCleaners = []buildCacheCleaner{
	// -cache is compiled objects, -modcache is downloaded modules. Both are
	// re-derivable; the modcache costs a re-download on the next build.
	{"go", []string{"clean", "-cache", "-modcache"}},
	// Removes packages no project references any more.
	{"pnpm", []string{"store", "prune"}},
	{"npm", []string{"cache", "clean", "--force"}},
	// Layer cache from image builds. Distinct from `docker system prune`,
	// which the standard cleanup already runs and which does NOT touch this.
	{"docker", []string{"builder", "prune", "-af"}},
}

// CleanBuildCaches clears re-derivable build caches. It is deliberately NOT
// part of the standard disk-cleanup: on a workstation these caches are the
// difference between a fast build and a slow one, and wiping them daily would
// be hostile. On a CI host they are usually the single largest consumer, and
// the standard cleanup does not touch them at all.
//
// That gap is not theoretical. On 2026-09-02 the nSelf CI host filled to 100%
// with zero bytes free, taking out every queued job in the org. The standard
// disk-cleanup reclaimed about 1 GB; the build caches on the same host held
// roughly 15 GB.
//
// A tool that is not installed is skipped, not an error.
func CleanBuildCaches() BuildCacheResult {
	result := BuildCacheResult{}

	before, err := GetDiskUsage()
	if err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("read disk usage (before): %w", err))
	}
	result.Before = before

	for _, c := range buildCacheCleaners {
		if _, lookErr := exec.LookPath(c.tool); lookErr != nil {
			result.Skipped = append(result.Skipped, c.tool)
			continue
		}
		if _, runErr := runCommand(c.tool, c.args...); runErr != nil {
			result.Errors = append(result.Errors,
				fmt.Errorf("%s %s: %w", c.tool, strings.Join(c.args, " "), runErr))
			continue
		}
		result.Ran = append(result.Ran, c.tool)
	}

	after, err := GetDiskUsage()
	if err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("read disk usage (after): %w", err))
	}
	result.After = after

	return result
}
