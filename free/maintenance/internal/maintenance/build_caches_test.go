package maintenance

import (
	"strings"
	"testing"
)

// NOTE: these tests must never let CleanBuildCaches actually run a cleaner.
// Doing so would wipe the Go build cache of whatever machine runs the suite,
// including a developer's laptop. Emptying PATH makes every LookPath fail, so
// every tool is skipped and no command is executed.
func TestCleanBuildCaches_SkipsToolsNotOnPath(t *testing.T) {
	t.Setenv("PATH", "")

	res := CleanBuildCaches()

	if len(res.Ran) != 0 {
		t.Fatalf("no tool is reachable with an empty PATH, but these ran: %v", res.Ran)
	}
	if len(res.Skipped) != len(buildCacheCleaners) {
		t.Fatalf("expected all %d tools skipped, got %d: %v",
			len(buildCacheCleaners), len(res.Skipped), res.Skipped)
	}
	for _, e := range res.Errors {
		// A missing tool is not an error; only disk-usage reads may fail here.
		if strings.Contains(e.Error(), "clean") || strings.Contains(e.Error(), "prune") {
			t.Fatalf("a missing tool was reported as an error: %v", e)
		}
	}
}

// The cleaners must delegate to each tool rather than deleting guessed paths.
// A path in this table would be a very expensive mistake on a shared host, and
// it is the kind of change that looks harmless in review.
func TestBuildCacheCleaners_UseToolNativeCommands(t *testing.T) {
	if len(buildCacheCleaners) == 0 {
		t.Fatal("cleaner table is empty")
	}
	banned := []string{"rm", "rmdir", "find", "shred", "unlink"}
	for _, c := range buildCacheCleaners {
		for _, b := range banned {
			if c.tool == b {
				t.Errorf("%q deletes paths directly; delegate to the tool instead", c.tool)
			}
		}
		if len(c.args) == 0 {
			t.Errorf("%q has no subcommand — a bare invocation is unlikely to clear a cache", c.tool)
		}
		for _, a := range c.args {
			if strings.HasPrefix(a, "/") || strings.HasPrefix(a, "~") {
				t.Errorf("%q takes a filesystem path %q; the tool should decide where its cache lives", c.tool, a)
			}
		}
	}
}

// docker system prune (the standard cleanup) does NOT clear the builder cache.
// If this entry is ever dropped, the CI-host case this flag exists for
// silently regresses.
func TestBuildCacheCleaners_IncludeDockerBuilder(t *testing.T) {
	for _, c := range buildCacheCleaners {
		if c.tool == "docker" && strings.Join(c.args, " ") == "builder prune -af" {
			return
		}
	}
	t.Fatal("docker builder prune is missing; `docker system prune` does not clear the build cache")
}
