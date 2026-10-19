package run

// Purpose: the real TrustClassifier (P7-TRUST-13): it gathers the git facts of the checkout through
// the exec command runner, classifies them with internal/trust/source, applies the machine-invoked
// cap, honours `--trust` and writes one audit row when `--trust owner` overrides the verdict. It
// also classifies a forge trigger (ClassifyForge), re-fetching the facts first. It is the only file
// of the package that may import providers.
// Constraints: a TrustRequest or forge facts in, a class, E625 or E687 out; registered from init(); git runs through Capture (report.Capture); no revision or no git
//          repository is the operator's own directory, as before this hook existed. The class is
//          min(classified, trust.machine_invoked_cap) when NSELF_INVOKED_BY is set; `--trust` may lower
//          the class, only `--trust owner` may raise it, and the audit row is written first: no row, no
//          run. A fact that cannot be gathered or re-fetched stays at its untrusted value.

import (
	"cmp"
	"context"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/trust"
	"github.com/nself-org/plugins/free/ci/internal/trust/source"
)

func init() { RegisterTrust(SourceTrust{}) }

// maxCommits bounds the commits the hook lists; more than this leaves the fact missing.
const maxCommits = 500

// SourceTrust is the registered classifier. The zero value uses report.Capture, the default state
// store, the environment and providers.RefetchFacts.
type SourceTrust struct {
	Capture   Capture
	OpenStore func() (*store.Store, error)
	Getenv    func(string) string
	Refetch   func(context.Context, model.TriggerFacts) (model.TriggerFacts, error)
}

// Classify implements TrustClassifier for a local run.
func (h SourceTrust) Classify(ctx context.Context, r TrustRequest) (model.TrustClass, error) {
	res, err := h.Decide(ctx, r)
	return res.Class, err
}

// Decide is Classify with the reasons: the class a run gets and why.
func (h SourceTrust) Decide(ctx context.Context, r TrustRequest) (source.Result, error) {
	res := source.Result{Class: "owner", Reasons: []string{source.ReasonLocalNoRevision}}
	if r.Revision != "" {
		res = source.ClassifyLocal(h.gather(ctx, r.Root, r.Revision))
	}
	capClass, machine, err := h.machineCap(ctx, r.Root)
	if err != nil {
		return res, err
	}
	if machine {
		res = source.ApplyCap(res, capClass)
	}
	req := model.TrustClass(r.Requested)
	if req == "" {
		return res, nil
	}
	if machine && trust.RankTrust(req) > trust.RankTrust(capClass) {
		return res, &roles.Error{Reason: "machine_cap", Msg: "--trust " + r.Requested + " is above trust.machine_invoked_cap " + string(capClass)}
	}
	if req == "owner" && res.Class != "owner" {
		if err := h.auditOverride(ctx, r, res); err != nil {
			return res, err
		}
		res.Class, res.Reasons = "owner", append(res.Reasons, source.ReasonLocalOverride)
		return res, nil
	}
	if trust.RankTrust(req) <= trust.RankTrust(res.Class) {
		res.Class = req
	}
	return res, nil // a request above the verdict is not granted: the coordinator names the mismatch
}

// ClassifyForge classifies a forge trigger: the facts are re-fetched from the API first, so a
// payload claim never stands in for a fact the operator's token can read.
func (h SourceTrust) ClassifyForge(ctx context.Context, root string, f model.TriggerFacts) (source.Result, error) {
	refetch := h.Refetch
	if refetch == nil {
		refetch = providers.RefetchFacts
	}
	fresh, ferr := refetch(ctx, f)
	if ferr == nil {
		f = fresh
	}
	res := source.Classify(f)
	if ferr != nil {
		res.Reasons = append(res.Reasons, source.ReasonRefetchFailed)
	}
	capClass, machine, err := h.machineCap(ctx, root)
	if err != nil {
		return res, err
	}
	if machine {
		res = source.ApplyCap(res, capClass)
	}
	return res, nil
}

func (h SourceTrust) getenv(k string) string {
	if h.Getenv != nil {
		return h.Getenv(k)
	}
	return os.Getenv(k)
}

func (h SourceTrust) capture() Capture {
	if h.Capture != nil {
		return h.Capture
	}
	return report.Capture
}

func (h SourceTrust) openStore() (*store.Store, error) {
	if h.OpenStore != nil {
		return h.OpenStore()
	}
	return OpenStore("")
}

// machineCap is the effective trust.machine_invoked_cap when the run is machine-invoked (Epic D27).
func (h SourceTrust) machineCap(ctx context.Context, root string) (model.TrustClass, bool, error) {
	if h.getenv("NSELF_INVOKED_BY") == "" {
		return "", false, nil
	}
	st, err := h.openStore()
	if err != nil {
		return "", true, fail(CodeTrust, "the machine-invoked trust cap could not be read: %v", err)
	}
	defer func() { _ = st.Close() }()
	root = cmp.Or(root, ".")
	res := policy.Resolver{Store: st, Run: policy.Runner(h.capture())}
	got, err := res.Resolve(ctx, policy.Repo{Root: root, Project: policy.ProjectIDGitCached(ctx, res.Run, root), Visibility: "unknown", Class: "owner"}, policy.Revision{Dir: root})
	if err != nil {
		return "", true, fail(CodeTrust, "the machine-invoked trust cap could not be resolved: %v", err)
	}
	v, _ := got.Values["trust.machine_invoked_cap"].(string)
	return trust.DecodeTrust(cmp.Or(v, "owner")), true, nil
}

// auditOverride writes the one audit row of a `--trust owner` that raised the class.
func (h SourceTrust) auditOverride(ctx context.Context, r TrustRequest, res source.Result) error {
	st, err := h.openStore()
	if err != nil {
		return fail(CodeTrust, "--trust owner needs an audit row and the store could not be opened: %v", err)
	}
	defer func() { _ = st.Close() }()
	ev := audit.Event{Actor: policy.LocalSubject, Action: "trust.override", Target: "revision:" + r.Revision,
		Detail: "requested=owner classified=" + string(res.Class) + " reasons=" + strings.Join(res.Reasons, ",")}
	if err := (&audit.Log{St: st}).Append(ctx, ev); err != nil {
		return fail(CodeTrust, "--trust owner needs an audit row and it could not be written: %v", err)
	}
	return nil
}

// gather reads the git facts of root at rev, the independent questions side by side. An answer git
// did not give goes to Missing, never a guess.
func (h SourceTrust) gather(ctx context.Context, root, rev string) source.GitFacts {
	g := source.GitFacts{Head: rev, ReflogCreated: map[string]string{}}
	git := func(args ...string) (string, int) {
		out, exit, err := h.capture()(ctx, append([]string{"git", "-c", "core.fsmonitor=false", "-C", root}, args...)...)
		if err != nil {
			return "", -1
		}
		return strings.TrimSpace(out), exit
	}
	var refs []string
	var symref, fetchHead string
	var remotesOK, reflogOK bool
	var wg sync.WaitGroup
	var mu sync.Mutex
	miss := func(key string) { mu.Lock(); g.Missing = append(g.Missing, key); mu.Unlock() }
	run := func(f func()) { wg.Add(1); go func() { defer wg.Done(); f() }() }
	run(func() {
		out, exit := git("for-each-ref", "--format=%(refname)", "refs/remotes", "refs/pull")
		refs, remotesOK = lines(out), exit == 0
	})
	run(func() { symref, _ = git("symbolic-ref", "-q", "refs/remotes/origin/HEAD") })
	run(func() {
		out, exit := git("rev-parse", "--is-inside-work-tree")
		g.NotARepo = exit != 0 && strings.Contains(out, "not a git repository")
	})
	run(func() { fetchHead, _ = git("rev-parse", "-q", "--verify", "FETCH_HEAD") })
	run(func() {
		// Every reflog, not HEAD's alone: a temporary worktree (`evidence verify`) has a HEAD log of its
		// own, while the branch logs that recorded the commit are shared.
		out, exit := git("reflog", "show", "--all", "--format=%H%x09%gs", "-n", "5000")
		reflogOK = exit == 0
		for _, l := range lines(out) {
			if sha, subject, ok := strings.Cut(l, "\t"); ok && !source.MadeHere(g.ReflogCreated[sha]) {
				g.ReflogCreated[sha] = subject
			}
		}
	})
	wg.Wait()
	if g.NotARepo {
		return g
	}
	g.NoRemotes = remotesOK && len(refs) == 0 && fetchHead == ""
	if !remotesOK {
		miss(source.MissingRemoteRefs)
	}
	if !reflogOK {
		miss(source.MissingReflog)
	}
	g.DefaultRemoteRef = defaultRef(symref, refs)
	if g.DefaultRemoteRef == "" && !g.NoRemotes {
		miss(source.MissingDefaultRef)
	}
	if g.NoRemotes || !remotesOK {
		return g
	}
	remote := "origin"
	if parts := strings.Split(g.DefaultRemoteRef, "/"); len(parts) > 3 {
		remote = parts[2]
	}
	notRemote := []string{"--not", "--remotes", "--glob=refs/pull/*"}
	if fetchHead != "" {
		notRemote = append(notRemote, "FETCH_HEAD")
	}
	run(func() {
		out, _ := git("for-each-ref", "--contains", rev, "--format=%(refname)", "refs/remotes/"+remote+"/")
		g.OnOriginRef = len(lines(out)) > 0
	})
	run(func() {
		out, exit := git("for-each-ref", "--contains", rev, "--format=%(refname)", "refs/remotes", "refs/pull")
		g.OnAnyRemoteRef = exit == 0 && len(lines(out)) > 0
		if !g.OnAnyRemoteRef && fetchHead != "" {
			_, exit = git("merge-base", "--is-ancestor", rev, "FETCH_HEAD")
			g.OnAnyRemoteRef = exit == 0
		}
	})
	run(func() {
		out, exit := git(append([]string{"rev-list", "-n", "501", rev}, notRemote...)...)
		g.Unpushed = commits(out)
		if exit != 0 || len(g.Unpushed) > maxCommits {
			miss(source.MissingUnpushed)
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
		run(func() {
			out, exit := git("rev-list", "-n", "501", rev, "--not", g.DefaultRemoteRef)
			g.OffDefault = commits(out)
			if exit != 0 || len(g.OffDefault) > maxCommits {
				miss(source.MissingUnpushed)
			}
		})
	}
	wg.Wait()
	return g
}

// defaultRef names the remote's default branch: origin/HEAD, else origin/main or origin/master.
func defaultRef(symref string, refs []string) string {
	if strings.HasPrefix(symref, "refs/remotes/") && strings.Count(symref, "/") >= 3 {
		return symref
	}
	for _, want := range []string{"refs/remotes/origin/main", "refs/remotes/origin/master"} {
		if slices.Contains(refs, want) {
			return want
		}
	}
	return ""
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
