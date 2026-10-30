package run

// Purpose: the patch half of local provenance (D-1322): a rewritten commit of a run (oldest reflog entry a
// rebase step, an amend or a cherry-pick) is created here only when its change has a source made here, so a
// replay of someone else's change is never counted as created here, even when nothing refers to the
// original any more (a deleted branch, an overwritten FETCH_HEAD, an expired reflog).
// Inputs:  the gathered GitFacts (OffDefault, ReflogCreated, DefaultRemoteRef, Head) and the amend pre-images
//          read from the reflog (amendPreImages).
// Outputs: GitFacts.ReplayedPatch, or MissingPatches when the comparison could not be made; grafted.
// Constraints: a source made here is a commit whose oldest entry is a creation other than a rewrite, a
//          rewrite already counted as made here, or a commit of the default branch that Head does not have;
//          a rewrite counts when its patch equals a source's, or (an amend) when the commit it amended
//          counts; a commit with no change lines adds no code and counts. The patch id ignores line numbers,
//          context and whitespace, so a replay onto a moved base still matches. Known cost, fail closed: a
//          rebase step whose conflicts you resolved, or a squash, changes the patch and is not counted.
//          More than maxPatchCommits commits on a side, or a git failure, is MissingPatches.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/trust/source"
)

// maxPatchCommits bounds the commits whose patches are compared on one side.
const maxPatchCommits = 5000

// patchLog is the log format both sides are read with: one NUL-led line with the commit id and its parents,
// then its patch.
var patchLog = []string{"-p", "--no-color", "--full-index", "--no-ext-diff", "--no-textconv", "--no-renames", "--format=%x00%H%x09%P"}

type gitFn func(args ...string) (string, int)

// replayedPatches fills g.ReplayedPatch with the rewritten commits of g.OffDefault that have no source made here.
func replayedPatches(git gitFn, g *source.GitFacts, amends map[string][]string) {
	var rewritten, made []string
	for _, c := range g.OffDefault {
		if source.Rewrite(g.ReflogCreated[c.SHA]) {
			rewritten = append(rewritten, c.SHA)
		}
	}
	if len(rewritten) == 0 {
		return
	}
	for sha, subject := range g.ReflogCreated {
		if source.MadeHere(subject) {
			made = append(made, sha)
		}
	}
	slices.Sort(made) // a stable argv: map order is random
	ids, parents, ok := logPatches(git, maxPatchCommits, append([]string{"--ignore-missing", "--no-walk=unsorted"}, made...))
	for _, sha := range rewritten {
		if _, have := ids[sha]; !have {
			ok = false
		}
	}
	if !ok {
		g.Missing = append(g.Missing, source.MissingPatches)
		return
	}
	own, created := map[string]bool{}, map[string]bool{}
	for _, sha := range made {
		if !source.Rewrite(g.ReflogCreated[sha]) {
			created[sha], own[ids[sha]] = true, true
		}
	}
	settle := func() bool { // true when every rewritten commit of the run counts as made here
		for changed := true; changed; {
			changed = false
			for _, sha := range made {
				subject, id := g.ReflogCreated[sha], ids[sha]
				if created[sha] || !source.Rewrite(subject) {
					continue
				}
				if id == "" || own[id] || amendedOwn(subject, sha, amends, parents, created) {
					created[sha], own[id], changed = true, true, true
				}
			}
		}
		return !slices.ContainsFunc(rewritten, func(sha string) bool { return !created[sha] })
	}
	if !settle() && g.DefaultRemoteRef != "" { // a backport: the default branch's own changes are sources too
		def, _, fine := logPatches(git, maxPatchCommits, []string{"--no-merges", g.DefaultRemoteRef, "--not", g.Head})
		if !fine {
			g.Missing = append(g.Missing, source.MissingPatches)
			return
		}
		for _, id := range def {
			own[id] = true
		}
		settle()
	}
	for _, sha := range rewritten {
		if !created[sha] {
			if g.ReplayedPatch == nil {
				g.ReplayedPatch = map[string]bool{}
			}
			g.ReplayedPatch[sha] = true
		}
	}
}

func amended(subject string) bool { return strings.HasPrefix(subject, "commit (amend)") }

// amendedOwn: sha is an amend of a commit counted as made here. The pre-image is the older neighbour in the
// reflog; an amend keeps the parents, so a neighbour with other parents (its own entry was deleted or
// expired) is not the pre-image and does not count.
func amendedOwn(subject, sha string, amends map[string][]string, parents map[string]string, created map[string]bool) bool {
	if !amended(subject) {
		return false
	}
	for _, p := range amends[sha] {
		if pp, known := parents[p]; known && created[p] && pp == parents[sha] {
			return true
		}
	}
	return false
}

// amendPreImages maps each commit an amend made to the candidates for the commit it replaced: the entry just
// older in each log that records the amend. Each log is listed newest first; the logs are grouped by ref
// here, whatever order they come in.
func amendPreImages(out string) map[string][]string {
	type entry struct{ sha, subject string }
	logs := map[string][]entry{}
	for _, l := range strings.Split(out, "\n") {
		f := strings.SplitN(strings.TrimSpace(l), "\t", 3)
		if len(f) != 3 || !fullSHA.MatchString(f[0]) {
			continue
		}
		ref := f[1]
		if at := strings.LastIndex(ref, "@{"); at >= 0 {
			ref = ref[:at]
		}
		logs[ref] = append(logs[ref], entry{f[0], f[2]})
	}
	pre := map[string][]string{}
	for _, es := range logs {
		for i := 0; i+1 < len(es); i++ {
			if amended(es[i].subject) {
				pre[es[i].sha] = append(pre[es[i].sha], es[i+1].sha)
			}
		}
	}
	return pre
}

// grafted reports whether a grafts file may rewrite the parents git reports: info/grafts of the repository
// or $GIT_GRAFT_FILE. A file that cannot be checked counts as present.
func (h SourceTrust) grafted(root, gitPath string) bool {
	for _, p := range []string{rootPath(root, gitPath), h.getenv("GIT_GRAFT_FILE")} {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err == nil || !errors.Is(err, fs.ErrNotExist) {
			return true
		}
	}
	return false
}

// logPatches runs `git log` with patchLog over args and returns the patch id and the parents of each
// commit; ok is false when git failed or more than limit commits came back.
func logPatches(git gitFn, limit int, args []string) (ids, parents map[string]string, ok bool) {
	out, exit := git(append(append([]string{"log", "-n", strconv.Itoa(limit + 1)}, patchLog...), args...)...)
	ids, parents = patchIDs(out)
	return ids, parents, exit == 0 && len(ids) <= limit
}

// patchIDs splits patchLog output into a patch id per commit ("" for a commit with no change lines) and
// its parents.
func patchIDs(out string) (ids, parents map[string]string) {
	ids, parents = map[string]string{}, map[string]string{}
	for _, block := range strings.Split(out, "\x00")[1:] {
		head, diff, _ := strings.Cut(block, "\n")
		sha, ps, _ := strings.Cut(strings.TrimSpace(head), "\t")
		if fullSHA.MatchString(sha) {
			ids[sha], parents[sha] = patchID(diff), strings.TrimSpace(ps)
		}
	}
	return ids, parents
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
