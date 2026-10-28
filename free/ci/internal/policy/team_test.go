package policy

// Purpose: tests of the team scope wiring (D-1284): the coordinator's team id (NSELF_CI_TEAM) reaches every
// resolve, a malformed id fails closed, and a user, team or project write is checked against every stored
// project and team row, not only the current repository's.
// Inputs:  a real SQLite store and the git fixtures of helpers_test.go.
// Outputs: pass or fail; no network. Fake values only.
// Constraints: tests only.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// The team row applies to a resolve that names no team once NSELF_CI_TEAM is set, and not before.
func TestResolveUsesTheCoordinatorTeam(t *testing.T) {
	f := newFixture(t)
	s := testStore(t)
	put(t, s, "team", "t1", "trust.network", `"none"`)
	r := resolver(t, f, s)
	repo := Repo{Root: f.work, Project: "proj", Visibility: "private", Class: "owner"}
	ctx := context.Background()

	t.Setenv(TeamEnv, "")
	got, err := r.Resolve(ctx, repo, Revision{Dir: f.work})
	if err != nil {
		t.Fatal(err)
	}
	if got.Sources["trust.network"] == SourceTeam {
		t.Fatalf("no team named, yet the team row applied: %v", got.Sources)
	}

	t.Setenv(TeamEnv, "t1")
	got, err = r.Resolve(ctx, repo, Revision{Dir: f.work})
	if err != nil {
		t.Fatal(err)
	}
	if got.Values["trust.network"] != "none" || got.Sources["trust.network"] != SourceTeam {
		t.Fatalf("team row not applied: %v %v", got.Values, got.Sources)
	}

	// An explicit team wins over the environment.
	repo.Team = "t2"
	got, err = r.Resolve(ctx, repo, Revision{Dir: f.work})
	if err != nil {
		t.Fatal(err)
	}
	if got.Sources["trust.network"] == SourceTeam {
		t.Fatalf("explicit team t2 has no rows, but the env team applied: %v", got.Sources)
	}
}

func TestMalformedTeamEnvFailsClosed(t *testing.T) {
	f := newFixture(t)
	r := resolver(t, f, testStore(t))
	for _, bad := range []string{"has space", "a/b", strings.Repeat("x", 65)} {
		t.Setenv(TeamEnv, bad)
		_, err := r.Resolve(context.Background(), Repo{Root: f.work, Project: "proj", Visibility: "private", Class: "owner"}, Revision{Dir: f.work})
		if err == nil || !strings.HasPrefix(err.Error(), "E672") {
			t.Fatalf("%q: want E672, got %v", bad, err)
		}
	}
}

// A user-scope write is checked against the rows of every project, so it cannot make another repository fail with E670.
func TestUserWriteIsCheckedAgainstOtherProjects(t *testing.T) {
	w, st, actor := testWriter(t)
	t.Setenv(TeamEnv, "")
	put(t, st, "project", "/other", "trust.network", `"internet"`)
	// "none" is the strictest network value; the project row of internet is looser than a user row of none, which
	// Resolve refuses with E670 in that other repository, so the write must be refused here.
	_, err := w.Apply(context.Background(), actor, Change{Op: OpSet, Scope: "user", Project: "/p", Key: "trust.network", Value: json.RawMessage(`"none"`)})
	if err == nil || !strings.HasPrefix(err.Error(), "E670") || !strings.Contains(err.Error(), "/other") {
		t.Fatalf("want E670 naming /other, got %v", err)
	}
	if rowCount(t, st, "user", LocalSubject) != 0 {
		t.Fatal("the refused write left a row")
	}
	// Once that project row is as strict, the same user row is accepted.
	put(t, st, "project", "/other", "trust.network", `"none"`)
	if _, err := w.Apply(context.Background(), actor, Change{Op: OpSet, Scope: "user", Project: "/p", Key: "trust.network", Value: json.RawMessage(`"none"`)}); err != nil {
		t.Fatalf("a compatible user row was refused: %v", err)
	}
}

// A team-scope write is checked against every project row; a project-scope write against every team row.
func TestTeamAndProjectWritesSeeTheOtherScopesRows(t *testing.T) {
	w, st, actor := testWriter(t)
	t.Setenv(TeamEnv, "")
	ctx := context.Background()
	put(t, st, "project", "/other", "trust.network", `"internet"`)
	_, err := w.Apply(ctx, actor, Change{Op: OpSet, Scope: "team", Team: "t1", Project: "/p", Key: "trust.network", Value: json.RawMessage(`"none"`)})
	if err == nil || !strings.HasPrefix(err.Error(), "E670") {
		t.Fatalf("a team row stricter than another project's row is E670, got %v", err)
	}
	put(t, st, "team", "t9", "trust.network", `"none"`)
	_, err = w.Apply(ctx, actor, Change{Op: OpSet, Scope: "project", Project: "/p", Key: "trust.network", Value: json.RawMessage(`"internet"`)})
	if err == nil || !strings.HasPrefix(err.Error(), "E670") {
		t.Fatalf("a project row looser than another team's row is E670, got %v", err)
	}
	// An unrelated, already-consistent row does not block a write.
	if _, err := w.Apply(ctx, actor, Change{Op: OpSet, Scope: "project", Project: "/p", Key: "trust.network", Value: json.RawMessage(`"none"`)}); err != nil {
		t.Fatalf("a compatible project row was refused: %v", err)
	}
}
