package run

// Purpose: the patch half of local provenance (D-1322): which rewritten commits of a run (oldest reflog entry
// a rebase step, an amend or a cherry-pick) replay a change someone else made, so they are not counted as
// created here: a rewrite whose patch arrived from elsewhere, and a cherry-pick whose patch was not made here.
// Inputs:  the gathered GitFacts (OffDefault, ReflogCreated, DefaultRemoteRef) and the FETCH_HEAD commits.
// Outputs: GitFacts.ReplayedPatch, or MissingPatches when the comparison could not be made.
// Constraints: the arrived side is every commit reachable from a remote-tracking ref, refs/pull/*, a FETCH_HEAD
//          line or a commit whose oldest reflog entry is an arrival, minus the default branch, minus every
//          commit whose own oldest entry is a creation (your own commit, pushed or not, is not arrived); the
//          patch id ignores line numbers, context lines and whitespace, so a replay onto a moved base still
//          matches; a cherry-pick counts as made here only when its patch is that of a commit made here (not a
//          cherry-pick) or of the default branch; more than maxPatchCommits commits on a side, or a git
//          failure, is MissingPatches. Residual: a rebase or amend of a commit nothing refers to any more.

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strconv"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/trust/source"
)

// maxPatchCommits bounds the arrived commits whose patches are compared.
const maxPatchCommits = 5000

// patchLog is the log format both sides are read with: one NUL-led commit id line, then its patch.
var patchLog = []string{"-p", "--no-color", "--full-index", "--no-ext-diff", "--no-textconv", "--no-renames", "--format=%x00%H"}

// replayedPatches fills g.ReplayedPatch for the rewritten commits of g.OffDefault: a rewrite whose patch
// arrived from elsewhere, and a cherry-pick whose patch is neither one made here nor one on the default
// branch (its source may be a commit nothing refers to any more, such as an overwritten FETCH_HEAD).
func replayedPatches(git func(args ...string) (string, int), g *source.GitFacts, fetched []string) {
	var rewritten []string
	picks := map[string]bool{}
	for _, c := range g.OffDefault {
		if subject := g.ReflogCreated[c.SHA]; source.Rewrite(subject) {
			rewritten = append(rewritten, c.SHA)
			picks[c.SHA] = cherryPick(subject)
		}
	}
	if len(rewritten) == 0 {
		return
	}
	mine, ok := logPatches(git, len(rewritten), append([]string{"--no-walk=unsorted"}, rewritten...))
	arrived, ok2 := arrivedPatches(git, g, fetched)
	own, ok3 := map[string]bool{}, true
	if slices.ContainsFunc(rewritten, func(sha string) bool { return picks[sha] }) {
		own, ok3 = ownPatches(git, g)
	}
	if !ok || !ok2 || !ok3 || len(mine) != len(rewritten) {
		g.Missing = append(g.Missing, source.MissingPatches)
		return
	}
	for _, sha := range rewritten {
		if id := mine[sha]; id != "" && (arrived[id] || picks[sha] && !own[id]) {
			if g.ReplayedPatch == nil {
				g.ReplayedPatch = map[string]bool{}
			}
			g.ReplayedPatch[sha] = true
		}
	}
}

// arrivedPatches are the patch ids of the commits that reached this repository from elsewhere and were not
// made here, off the default branch.
func arrivedPatches(git func(args ...string) (string, int), g *source.GitFacts, fetched []string) (map[string]bool, bool) {
	var tips []string
	for sha, subject := range g.ReflogCreated {
		if !source.MadeHere(subject) {
			tips = append(tips, sha)
		}
	}
	slices.Sort(tips) // a stable argv: map order is random
	// --ignore-missing: a FETCH_HEAD commit gc has pruned is no longer anything to compare with
	args := append(append([]string{"--ignore-missing", "--no-merges", "--remotes", "--glob=refs/pull/*"}, fetched...), tips...)
	if g.DefaultRemoteRef != "" {
		args = append(args, "--not", g.DefaultRemoteRef)
	}
	ids, ok := logPatches(git, maxPatchCommits, args)
	arrived := map[string]bool{}
	for sha, id := range ids {
		if id != "" && !source.MadeHere(g.ReflogCreated[sha]) {
			arrived[id] = true
		}
	}
	return arrived, ok
}

// ownPatches are the patch ids a cherry-pick may come from and still be created here: a commit whose oldest
// entry is a creation other than a cherry-pick, and a commit of the default branch that Head does not have.
func ownPatches(git func(args ...string) (string, int), g *source.GitFacts) (map[string]bool, bool) {
	var made []string
	for sha, subject := range g.ReflogCreated {
		if source.MadeHere(subject) && !cherryPick(subject) {
			made = append(made, sha)
		}
	}
	slices.Sort(made)
	own, ok := map[string]bool{}, true
	add := func(ids map[string]string, fine bool) {
		ok = ok && fine
		for _, id := range ids {
			if id != "" {
				own[id] = true
			}
		}
	}
	if len(made) > 0 {
		add(logPatches(git, maxPatchCommits, append([]string{"--ignore-missing", "--no-walk=unsorted"}, made...)))
	}
	if g.DefaultRemoteRef != "" {
		add(logPatches(git, maxPatchCommits, []string{"--no-merges", g.DefaultRemoteRef, "--not", g.Head}))
	}
	return own, ok
}

// logPatches runs `git log` with patchLog over args and returns the patch id of each commit; ok is false when
// git failed or more than limit commits came back.
func logPatches(git func(args ...string) (string, int), limit int, args []string) (map[string]string, bool) {
	out, exit := git(append(append([]string{"log", "-n", strconv.Itoa(limit + 1)}, patchLog...), args...)...)
	ids := patchIDs(out)
	return ids, exit == 0 && len(ids) <= limit
}

func cherryPick(subject string) bool {
	return strings.HasPrefix(strings.TrimSpace(subject), "cherry-pick")
}

// patchIDs splits patchLog output into a patch id per commit ("" for a commit with no change lines).
func patchIDs(out string) map[string]string {
	ids := map[string]string{}
	for _, block := range strings.Split(out, "\x00")[1:] {
		sha, diff, _ := strings.Cut(block, "\n")
		if sha = strings.TrimSpace(sha); fullSHA.MatchString(sha) {
			ids[sha] = patchID(diff)
		}
	}
	return ids
}

// patchID hashes the changes of one commit the way `git patch-id --stable` treats them, and looser: per file
// its header lines and its added and removed lines with all whitespace removed (no hunk positions, no index
// line, no context), the file hashes sorted, so file order and a moved base do not change it.
func patchID(diff string) string {
	var files []string
	var cur []string
	flush := func() {
		if len(cur) > 1 {
			sum := sha256.Sum256([]byte(strings.Join(cur, "\n")))
			files = append(files, hex.EncodeToString(sum[:]))
		}
		cur = nil
	}
	for _, l := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(l, "diff --git "):
			flush()
			cur = []string{l}
		case cur == nil, l == "", strings.HasPrefix(l, "index "), strings.HasPrefix(l, "@@"), strings.HasPrefix(l, " "):
		case strings.HasPrefix(l, "+++ "), strings.HasPrefix(l, "--- "):
			cur = append(cur, l)
		default: // + and - lines, mode lines, "Binary files ... differ"
			cur = append(cur, strings.Join(strings.Fields(l), ""))
		}
	}
	flush()
	if len(files) == 0 {
		return ""
	}
	slices.Sort(files)
	sum := sha256.Sum256([]byte(strings.Join(files, "\n")))
	return hex.EncodeToString(sum[:])
}
