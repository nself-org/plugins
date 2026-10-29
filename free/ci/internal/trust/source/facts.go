package source

// Purpose: the facts source trust is decided from (Epic D17): the git facts of a local run
// (GitFacts), the Result both classifiers return, and the helpers that read a normalized
// model.TriggerFacts without trusting a claim the forge payload made about itself.
// Inputs:  GitFacts gathered by internal/run/trust_hook.go; model.TriggerFacts from P7-HOST-01.
// Outputs: Result values; the Reason keys; Authoritative and the lockfile/bot tests.
// Constraints: pure (no os, no exec, no net, no database/sql): git is run in the hook, never here.
//          A commit author is never a field of any type in this package: author name and email are
//          chosen by whoever made the commit, so they are not evidence (review F3).

import (
	"path"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Reason keys of a Result, in the order the classifiers add them.
const (
	ReasonAuthNotPerRepo   = "source.auth_not_per_repo"
	ReasonActor            = "source.actor_permission"
	ReasonActorUnknown     = "source.actor_unknown"
	ReasonFork             = "source.fork"
	ReasonForkUnknown      = "source.head_repo_unknown"
	ReasonProvenance       = "source.revision_provenance"
	ReasonUnreachable      = "source.sha_unreachable"
	ReasonBot              = "source.bot"
	ReasonLockfileOnly     = "source.lockfile_only"
	ReasonHeadChanged      = "source.head_changed"
	ReasonRunnerDemand     = "source.runner_demand"
	ReasonRefetchFailed    = "source.refetch_failed"
	ReasonMachineCap       = "source.machine_invoked_cap"
	ReasonLocalReachable   = "local.reachable_from_default"
	ReasonLocalCreated     = "local.created_here"
	ReasonLocalOnOrigin    = "local.on_origin_ref"
	ReasonLocalForeign     = "local.not_created_here"
	ReasonLocalNoHead      = "local.no_head"
	ReasonLocalNoRevision  = "local.no_revision"
	ReasonLocalNoRemote    = "local.no_remote"
	ReasonLocalNoGit       = "local.no_git"
	ReasonLocalGitBroken   = "local.git_unavailable"
	ReasonLocalBadRevision = "local.bad_revision"
	ReasonLocalInProgress  = "local.operation_in_progress"
	ReasonLocalIncomplete  = "local.facts_incomplete"
	ReasonLocalOverride    = "local.trust_owner_override"
	ReasonLocalReplayed    = "local.replayed_arrived_patch"
)

// Result is a classification: the class, why, and the side facts the callers need.
type Result struct {
	Class            model.TrustClass
	Reasons          []string // ordered, deterministic
	RequiresApproval bool     // a first-time contributor's run waits for an approval (P7-TRUST-24)
	VisibilityZone   model.PrivacyZone
}

// Commit is one commit of the local history. It carries the id only: see the package constraints.
type Commit struct{ SHA string }

// Missing keys: facts the hook could not gather. A missing fact is never read as a good answer.
const (
	MissingDefaultRef = "default_ref"
	MissingReachable  = "reachable"
	MissingRemoteRefs = "remote_refs"
	MissingCommits    = "commits"
	MissingReflog     = "reflog"
	MissingPatches    = "patches"
)

// GitFacts are the git facts of the checkout a local run is bound to.
type GitFacts struct {
	Head string // the revision the run is bound to
	// NotARepo: git itself says the directory is not inside a repository, so there is no history to
	// classify and the files are the operator's own directory.
	NotARepo         bool
	DefaultRemoteRef string // e.g. refs/remotes/origin/main; empty when it cannot be named
	// ReachableFromDefault: Head is an ancestor of (or equal to) DefaultRemoteRef.
	ReachableFromDefault bool
	// OnOriginRef: Head is contained in a branch of the remote that owns DefaultRemoteRef.
	OnOriginRef bool
	// NoRemotes: no remote is configured and nothing was ever fetched (no remote-tracking ref, no
	// refs/pull/*, no FETCH_HEAD).
	NoRemotes bool
	// NeverFetched: a remote is configured but nothing was ever fetched from it. Every commit of Head
	// is then in OffDefault.
	NeverFetched bool
	// InProgress: a merge, squash, cherry-pick, revert or rebase is unfinished, so the tree holds
	// content that no commit of Head accounts for.
	InProgress bool
	// OffDefault are the commits of Head that DefaultRemoteRef does not contain (all of them when
	// NeverFetched).
	OffDefault []Commit
	// ReflogCreated maps a commit to the subject of its OLDEST reflog entry in any log (OldestEntries),
	// the way it first reached this repository, for example "commit: fix" or "branch: Created from FETCH_HEAD".
	ReflogCreated map[string]string
	// ReplayedPatch marks the commits of OffDefault whose oldest entry is a rewrite (Rewrite: a rebase step,
	// an amend, a cherry-pick) and whose patch equals that of a commit that arrived here and was not made
	// here (reachable from a remote-tracking ref, refs/pull/*, a FETCH_HEAD line or a reflog arrival entry).
	// Such a commit is someone else's change replayed, not created here (D-1322).
	ReplayedPatch map[string]bool
	// Missing lists the facts that could not be gathered (the Missing* keys).
	Missing []string
}

// ReflogEntry is one line of a reflog: a commit, when the entry was written and why.
type ReflogEntry struct {
	SHA     string
	Unix    int64
	Subject string
}

// OldestEntries maps each commit to the subject of its oldest entry. Entries are timed to the second
// and the logs cannot be ordered within one, so among the earliest a creation (MadeHere) wins: a commit
// that only arrived (fetch, checkout) has no creation entry to win with, and a fast-forward is never one.
func OldestEntries(es []ReflogEntry) map[string]string {
	best := make(map[string]ReflogEntry, len(es))
	for _, e := range es {
		b, seen := best[e.SHA]
		if !seen || e.Unix < b.Unix || (e.Unix == b.Unix && !MadeHere(b.Subject) && MadeHere(e.Subject)) {
			best[e.SHA] = e
		}
	}
	out := make(map[string]string, len(best))
	for sha, e := range best {
		out[sha] = e.Subject
	}
	return out
}

func (g GitFacts) missing(key string) bool {
	for _, m := range g.Missing {
		if m == key {
			return true
		}
	}
	return false
}

// Authoritative reports whether a fact may RAISE trust: only a value the forge API (or this
// machine) gave. A payload value, or one a caller supplied, can only lower trust.
func Authoritative(source string) bool { return source == "api" || source == "local" }

func trusted[T any](f model.Fact[T]) (T, bool) {
	var zero T
	if f.Value == nil || !Authoritative(f.Source) {
		return zero, false
	}
	return *f.Value, true
}

func claimed[T any](f model.Fact[T]) (T, bool) {
	var zero T
	if f.Value == nil {
		return zero, false
	}
	return *f.Value, true
}

var lockfiles = map[string]bool{
	".terraform.lock.hcl": true, "go.work.sum": true, "pixi.lock": true, "conda-lock.yml": true,
	"paket.lock": true, "rebar.lock": true, "module.bazel.lock": true, "cartfile.resolved": true,
	"shard.lock": true, "stack.yaml.lock": true, "cabal.project.freeze": true, "pnpm-lock.yml": true,
	"package-lock.json": true, "npm-shrinkwrap.json": true, "pnpm-lock.yaml": true, "yarn.lock": true,
	"bun.lockb": true, "bun.lock": true, "go.sum": true, "cargo.lock": true, "gemfile.lock": true,
	"poetry.lock": true, "pipfile.lock": true, "uv.lock": true, "composer.lock": true, "pubspec.lock": true,
	"mix.lock": true, "podfile.lock": true, "packages.lock.json": true, "gradle.lockfile": true,
	"flake.lock": true, "pdm.lock": true, "deno.lock": true, "package.resolved": true,
}

// LockfileOnly reports whether files is non-empty and every file is a dependency lockfile.
func LockfileOnly(files []string) bool {
	if len(files) == 0 {
		return false
	}
	for _, f := range files {
		normalized := strings.ToLower(strings.ReplaceAll(f, `\`, "/"))
		gradle := strings.HasSuffix(normalized, ".lockfile") && strings.Contains("/"+normalized, "/gradle/dependency-locks/")
		if !lockfiles[path.Base(normalized)] && !gradle {
			return false
		}
	}
	return true
}

// IsBot reports whether login is an automation account: a [bot] login, Dependabot or Renovate.
func IsBot(login string) bool {
	l := strings.ToLower(strings.TrimSpace(login))
	if strings.HasSuffix(l, "[bot]") || strings.HasSuffix(l, "-bot") {
		return true
	}
	for _, p := range []string{"dependabot", "renovate"} {
		if l == p || strings.HasPrefix(l, p+"[") || strings.HasPrefix(l, p+"-") || strings.HasPrefix(l, "app/"+p) {
			return true
		}
	}
	return false
}
