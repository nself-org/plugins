package source_test

// D-1322: a rebase, amend or cherry-pick of someone else's commit makes a new commit here, but it replays
// their patch, so it is not created here. Pure facts first, then real git: the stranger's replays stay below
// owner, and the operator's own rewrites (pushed or not) stay owner.

import (
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/trust/source"
)

func TestClassifyRewrite(t *testing.T) {
	for subject, want := range map[string]bool{
		"commit (amend): w": true, "rebase (pick): w": true, "rebase -i (pick): w": true, "rebase (squash): w": true, "rebase (continue): w": true,
		"pull -q --rebase origin main (pick): w": true, "cherry-pick: w": true, "cherry-pick (continue): w": true,
		"commit: w": false, "commit (initial): w": false, "commit (merge): w": false, "revert: Revert \"w\"": false,
		"merge feat: Merge made by the 'ort' strategy.": false, "pull origin main: Merge made by the 'ort' strategy.": false,
		"cherry-pick: fast-forward": false, "rebase (finish): refs/heads/x onto 0123": false, "checkout: moving from a to b": false, "": false,
	} {
		if got := source.Rewrite(subject); got != want {
			t.Errorf("Rewrite(%q) = %v", subject, got)
		}
	}
}

func TestClassifyLocalReplayedPatch(t *testing.T) {
	const def = "refs/remotes/origin/main"
	rewritten := map[string]string{"a": "commit: base", "b": "rebase (pick): w"}
	for _, tc := range []struct {
		name   string
		g      source.GitFacts
		want   model.TrustClass
		reason string
	}{
		{"a rewrite of an own patch", source.GitFacts{Head: "b", DefaultRemoteRef: def, OffDefault: c("a", "b"), ReflogCreated: rewritten}, "owner", source.ReasonLocalCreated},
		{"a rewrite of an arrived patch", source.GitFacts{Head: "b", DefaultRemoteRef: def, OffDefault: c("a", "b"), ReflogCreated: rewritten, ReplayedPatch: map[string]bool{"b": true}},
			"untrusted", source.ReasonLocalReplayed},
		{"a replayed patch pushed to origin", source.GitFacts{Head: "b", DefaultRemoteRef: def, OnOriginRef: true, OffDefault: c("a", "b"), ReflogCreated: rewritten,
			ReplayedPatch: map[string]bool{"b": true}}, "collaborator", source.ReasonLocalOnOrigin},
		{"patches not compared closes the owner path for a rewrite", source.GitFacts{Head: "b", DefaultRemoteRef: def, OffDefault: c("a", "b"), ReflogCreated: rewritten,
			Missing: []string{source.MissingPatches}}, "untrusted", source.ReasonLocalIncomplete},
		{"patches not compared do not matter without a rewrite", source.GitFacts{Head: "a", DefaultRemoteRef: def, OffDefault: c("a"), ReflogCreated: rewritten,
			Missing: []string{source.MissingPatches}}, "owner", source.ReasonLocalCreated},
	} {
		got := source.ClassifyLocal(tc.g)
		if got.Class != tc.want || !contains(got.Reasons, tc.reason) {
			t.Errorf("%s: %s %v, want %s with %s", tc.name, got.Class, got.Reasons, tc.want, tc.reason)
		}
	}
}

// advance pushes one commit by someone else to origin/main and fetches it, so a replay is not a fast-forward.
func (x *fx) advance() {
	x.t.Helper()
	other := filepath.Join(x.tmp, "other")
	git(x.t, x.tmp, "clone", "-q", x.origin, other)
	git(x.t, other, "config", "user.name", "Other")
	git(x.t, other, "config", "user.email", "other@example.test")
	git(x.t, other, "config", "commit.gpgsign", "false")
	x.commit(other, "newer.txt")
	git(x.t, other, "push", "-q", "origin", "main")
	git(x.t, x.op, "fetch", "-q", "origin")
}

func (x *fx) fetchStranger() string {
	x.t.Helper()
	git(x.t, x.op, "fetch", "-q", x.stranger, "pr")
	return git(x.t, x.op, "rev-parse", "FETCH_HEAD")
}

func TestClassifyGitReplays(t *testing.T) {
	check := func(name string, setup func(*fx), want model.TrustClass, reason string) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			x := newFx(t)
			setup(x)
			got := x.decide("", run.SourceTrust{})
			if got.Class != want || !contains(got.Reasons, reason) {
				t.Fatalf("%s %v, want %s with %s", got.Class, got.Reasons, want, reason)
			}
		})
	}
	// The stranger's patch, replayed by the operator.
	check("stranger branch rebased onto newer main", func(x *fx) {
		sha := x.fetchStranger()
		x.advance()
		git(x.t, x.op, "checkout", "-q", "-b", "x", sha)
		git(x.t, x.op, "rebase", "-q", "origin/main")
	}, "untrusted", source.ReasonLocalReplayed)
	check("stranger commit amended (message only)", func(x *fx) {
		git(x.t, x.op, "checkout", "-q", "-b", "pr", x.fetchStranger())
		git(x.t, x.op, "commit", "-q", "--amend", "-m", "tidy message")
	}, "untrusted", source.ReasonLocalReplayed)
	check("stranger commit cherry-picked while FETCH_HEAD holds it", func(x *fx) {
		sha := x.fetchStranger()
		git(x.t, x.op, "checkout", "-q", "-b", "y")
		x.commit(x.op, "mine.txt")
		git(x.t, x.op, "cherry-pick", sha)
	}, "untrusted", source.ReasonLocalReplayed)
	check("stranger commit cherry-picked after FETCH_HEAD moved on", func(x *fx) {
		sha := x.fetchStranger()
		x.advance()
		git(x.t, x.op, "checkout", "-q", "-b", "y", "origin/main")
		git(x.t, x.op, "cherry-pick", sha)
	}, "untrusted", source.ReasonLocalReplayed)
	check("a collaborator's origin branch rebased onto newer main", func(x *fx) {
		git(x.t, x.stranger, "push", "-q", "origin", "pr")
		x.advance()
		git(x.t, x.op, "checkout", "-q", "-b", "pr", "origin/pr")
		git(x.t, x.op, "rebase", "-q", "origin/main")
	}, "untrusted", source.ReasonLocalReplayed)
	// The operator's own work, rewritten: still created here.
	check("own pushed branch rebased onto newer main", func(x *fx) {
		git(x.t, x.op, "checkout", "-q", "-b", "feat")
		x.commit(x.op, "one.txt")
		git(x.t, x.op, "push", "-q", "-u", "origin", "feat")
		x.advance()
		git(x.t, x.op, "rebase", "-q", "origin/main")
	}, "owner", source.ReasonLocalCreated)
	check("own commit amended with a content change", func(x *fx) {
		git(x.t, x.op, "checkout", "-q", "-b", "feat")
		x.commit(x.op, "one.txt")
		x.commit(x.op, "two.txt")
		git(x.t, x.op, "reset", "-q", "--soft", "HEAD~1")
		git(x.t, x.op, "commit", "-q", "--amend", "-m", "one and two")
	}, "owner", source.ReasonLocalCreated)
	check("own commit cherry-picked to another branch", func(x *fx) {
		git(x.t, x.op, "checkout", "-q", "-b", "feat")
		sha := x.commit(x.op, "one.txt")
		git(x.t, x.op, "checkout", "-q", "-b", "other", "origin/main")
		git(x.t, x.op, "cherry-pick", sha)
	}, "owner", source.ReasonLocalCreated)
	check("a default-branch commit cherry-picked onto an older branch", func(x *fx) {
		git(x.t, x.op, "checkout", "-q", "-b", "release")
		x.commit(x.op, "rel.txt")
		x.advance()
		git(x.t, x.op, "cherry-pick", "origin/main")
	}, "owner", source.ReasonLocalCreated)
}
