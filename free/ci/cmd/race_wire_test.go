package main

import (
	"fmt"
	"strings"
	"testing"

	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/where"
)

// TestWire07WhereAllReachesRace drives `nself ci --where all` through the real
// V15 entry by argv: the race hook linked into the binary is registered, so
// --where all resolves to the race target instead of E797.
// No import of internal/race here: a test import would link it and hide a
// missing wire in the cmd composition root.
func TestWire07WhereAllReachesRace(t *testing.T) {
	if got := fmt.Sprintf("%T", cirun.Registered.Auto.Race); got != "*race.Hook" {
		t.Fatalf("registered race hook is %s, want *race.Hook (internal/race not linked into cmd)", got)
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("NSELF_V15", "1")
	t.Setenv("NSELF_CI_HOME", t.TempDir())
	t.Setenv("NSELF_CI_REPO", t.TempDir())
	oldV := whereValue
	t.Cleanup(func() { whereValue = oldV })

	_, _, errs := capture(t, func() int { return runV15([]string{"--where", "all", "--no-status"}) })
	if strings.Contains(errs, "E797") || strings.Contains(errs, "race not available") {
		t.Fatalf("--where all refused: %s", errs)
	}
	if whereChosen.Kind != where.KindAll {
		t.Fatalf("--where all resolved to %q, want %q (stderr: %s)", whereChosen.Kind, where.KindAll, errs)
	}
}
