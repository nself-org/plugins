package run

// Purpose: the git half of SourceTrust (P7-TRUST-13): the questions asked of a checkout through the
// exec command runner, and the GitFacts they make.
// Inputs:  the repository root and the revision a run is bound to.
// Outputs: source.GitFacts; a verdict for a checkout with no revision.
// Constraints: git runs with --no-replace-objects (a `git replace` ref must not rewrite the history
//          under test); only refs/remotes/origin/ names the default branch; every FETCH_HEAD line counts,
//          not just the first; an unfinished merge, squash, cherry-pick, revert or rebase is reported;
//          a git failure is listed in Missing, never read as an answer; the reflog is read from every
//          log (a linked worktree has a HEAD log of its own) and each commit keeps its OLDEST entry.

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/trust/source"
)

// maxCommits bounds the commits listed; a longer list leaves the fact missing.
const maxCommits = 500

// prDirs are remote-tracking namespaces that hold other people's pull requests, not the remote's branches.
var prDirs = []string{"pr", "pull", "pulls", "mr", "merge-requests"}

func (h SourceTrust) git(ctx context.Context, root string) func(args ...string) (string, int) {
	return func(args ...string) (string, int) {
		out, exit, err := h.capture()(ctx, append([]string{"git", "--no-replace-objects", "-c", "core.fsmonitor=false", "-C", root}, args...)...)
		if err != nil {
			return "", -1
		}
		return strings.TrimSpace(out), exit
	}
}

// noRevision classifies a checkout GitRevision found no commit in: a repository without a commit and a
// directory that is no repository are the operator's own; a git that cannot run there (a repository of
// another user, "dubious ownership") says nothing, so it is not.
func (h SourceTrust) noRevision(ctx context.Context, root string) source.Result {
	out, exit := h.git(ctx, root)("rev-parse", "--git-dir")
	switch {
	case exit == 0:
		if _, head := h.git(ctx, root)("rev-parse", "-q", "--verify", "HEAD"); head != 1 {
			break // HEAD resolves, so the earlier failure was not "no commit yet"
		}
		return source.Result{Class: "owner", Reasons: []string{source.ReasonLocalNoRevision}}
	case strings.Contains(out, "not a git repository"):
		return source.Result{Class: "owner", Reasons: []string{source.ReasonLocalNoGit}}
	}
	return source.Result{Class: "untrusted", Reasons: []string{source.ReasonLocalGitBroken}}
}

// gather reads the git facts of root at rev, the independent questions side by side.
func (h SourceTrust) gather(ctx context.Context, root, rev string) source.GitFacts {
	g := source.GitFacts{Head: rev}
	git := h.git(ctx, root)
	var mu sync.Mutex
	miss := func(key string) { mu.Lock(); g.Missing = append(g.Missing, key); mu.Unlock() }
	var wg sync.WaitGroup
	run := func(f func()) { wg.Add(1); go func() { defer wg.Done(); f() }() }
	var refs, paths, remotes []string
	var symref string
	var refsOK, remotesOK, pathsOK bool
	var entries []source.ReflogEntry
	run(func() {
		out, exit := git("for-each-ref", "--format=%(refname)", "refs/remotes", "refs/pull")
		refs, refsOK = lines(out), exit == 0
	})
	run(func() { symref, _ = git("symbolic-ref", "-q", "refs/remotes/origin/HEAD") })
	run(func() { out, exit := git("remote"); remotes, remotesOK = lines(out), exit == 0 })
	run(func() {
		out, exit := git("rev-parse", "--is-inside-work-tree")
		g.NotARepo = exit != 0 && strings.Contains(out, "not a git repository")
	})
	run(func() {
		out, exit := git("rev-parse", "--git-path", "FETCH_HEAD", "--git-path", "SQUASH_MSG", "--git-path", "MERGE_HEAD", "--git-path", "CHERRY_PICK_HEAD",
			"--git-path", "REVERT_HEAD", "--git-path", "rebase-merge", "--git-path", "rebase-apply")
		paths, pathsOK = lines(out), exit == 0 && len(lines(out)) == 7
	})
	run(func() {
		out, exit := git("reflog", "show", "--all", "--date=unix", "--format=%H%x09%gd%x09%gs", "-n", "5000")
		if exit != 0 {
			miss(source.MissingReflog)
		}
		entries = reflogEntries(out)
	})
	wg.Wait()
	if g.NotARepo {
		return g
	}
	g.ReflogCreated = source.OldestEntries(entries)
	var fetched []string
	if pathsOK {
		var ok bool
		fetched, ok = fetchHeads(root, paths[0])
		if !ok {
			pathsOK = false
		}
		for _, m := range paths[1:] {
			if _, err := os.Stat(rootPath(root, m)); err == nil {
				g.InProgress = true
			}
		}
	}
	if !pathsOK || !refsOK || !remotesOK {
		miss(source.MissingRemoteRefs)
		return g
	}
	g.NoRemotes = len(refs) == 0 && len(fetched) == 0 && len(remotes) == 0
	g.NeverFetched = len(refs) == 0 && len(fetched) == 0 && len(remotes) > 0
	g.DefaultRemoteRef = defaultRef(symref, refs)
	if g.NoRemotes {
		return g
	}
	if g.DefaultRemoteRef == "" && !g.NeverFetched {
		miss(source.MissingDefaultRef)
	}
	listed := func(args ...string) []source.Commit {
		out, exit := git(args...)
		cs := commits(out)
		if exit != 0 || len(cs) > maxCommits {
			miss(source.MissingCommits)
		}
		return cs
	}
	run(func() {
		out, exit := git("config", "--get-all", "remote.origin.fetch")
		skip := excludedRemoteDirs(lines(out))
		out, exit2 := git("for-each-ref", "--contains", rev, "--format=%(refname)", "refs/remotes/origin/")
		if exit2 != 0 || exit < 0 || exit > 1 {
			miss(source.MissingRemoteRefs)
		}
		for _, r := range lines(out) {
			if r != "refs/remotes/origin/HEAD" && !slices.ContainsFunc(skip, func(p string) bool { return strings.HasPrefix(r, p) }) {
				g.OnOriginRef = true
			}
		}
	})
	if g.DefaultRemoteRef != "" {
		run(func() {
			_, exit := git("merge-base", "--is-ancestor", rev, g.DefaultRemoteRef)
			g.ReachableFromDefault = exit == 0
			if exit != 0 && exit != 1 {
				miss(source.MissingReachable)
			}
		})
		run(func() { g.OffDefault = listed("rev-list", "-n", "501", rev, "--not", g.DefaultRemoteRef) })
	} else if g.NeverFetched {
		run(func() { g.OffDefault = listed("rev-list", "-n", "501", rev) })
	}
	wg.Wait()
	h.verifyRewrites(ctx, root, &g, append(refs, fetched...))
	return g
}

// defaultRef names the remote's default branch: origin/HEAD, else origin/main or origin/master. Only
// the remote named origin counts: origin/HEAD may be pointed anywhere.
func defaultRef(symref string, refs []string) string {
	if strings.HasPrefix(symref, "refs/remotes/origin/") && !strings.HasSuffix(symref, "/HEAD") && slices.Contains(refs, symref) {
		return symref
	}
	for _, want := range []string{"refs/remotes/origin/main", "refs/remotes/origin/master"} {
		if slices.Contains(refs, want) {
			return want
		}
	}
	return ""
}

// excludedRemoteDirs lists the refs/remotes/origin/ prefixes that hold pull requests: the usual names
// and every destination the origin's fetch refspecs map from something other than refs/heads/*.
func excludedRemoteDirs(refspecs []string) []string {
	var skip []string
	for _, d := range prDirs {
		skip = append(skip, "refs/remotes/origin/"+d+"/")
	}
	for _, spec := range refspecs {
		src, dst, ok := strings.Cut(strings.TrimPrefix(spec, "+"), ":")
		if ok && !strings.HasPrefix(src, "refs/heads/") {
			skip = append(skip, strings.TrimSuffix(dst, "*"))
		}
	}
	return skip
}

func rootPath(root, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(root, p)
}

// fetchHeads returns every commit named in FETCH_HEAD (a multi-ref fetch writes one line each).
func fetchHeads(root, path string) ([]string, bool) {
	b, err := os.ReadFile(rootPath(root, path))
	if os.IsNotExist(err) {
		return nil, true
	}
	if err != nil {
		return nil, false
	}
	var shas []string
	for _, l := range lines(string(b)) {
		sha, _, _ := strings.Cut(l, "\t")
		if fullSHA.MatchString(sha) && !slices.Contains(shas, sha) {
			shas = append(shas, sha)
		}
	}
	return shas, true
}

func reflogEntries(out string) (es []source.ReflogEntry) {
	for _, l := range lines(out) {
		f := strings.SplitN(l, "\t", 3)
		if len(f) != 3 || !fullSHA.MatchString(f[0]) {
			continue
		}
		at := strings.LastIndex(f[1], "@{")
		if at < 0 {
			continue
		}
		ts, err := strconv.ParseInt(strings.TrimSuffix(f[1][at+2:], "}"), 10, 64)
		if err != nil {
			continue
		}
		es = append(es, source.ReflogEntry{SHA: f[0], Unix: ts, Subject: f[2]})
	}
	return es
}

func lines(out string) (ls []string) {
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			ls = append(ls, l)
		}
	}
	return ls
}

func commits(out string) (cs []source.Commit) {
	for _, l := range lines(out) {
		if fullSHA.MatchString(l) {
			cs = append(cs, source.Commit{SHA: l})
		}
	}
	return cs
}
