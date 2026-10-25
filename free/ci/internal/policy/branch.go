package policy

// Purpose: read the protected default branch's `policy:` block (Epic D15).
// The branch is read at the fetched refs/remotes/origin/<default> tip with
// `git show <sha>:.nself/ci.yaml`, never from the worktree.
// Inputs: the repository root and a Runner (the exec funnel).
// Outputs: a Branch: default name, tip sha, fetch state and the flat policy.
// Constraints: every git call goes through the Runner (only internal/exec
// starts processes); the only network access is one `git fetch` of the
// default branch; a failed fetch falls back to the existing remote-tracking
// ref and reports FetchStale; no origin means no branch scope (FetchNone).

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/config"
)

// Runner runs a process and returns merged output and its exit code. A
// non-zero exit is not an error; err is set only when it could not run.
type Runner func(ctx context.Context, argv ...string) (out string, exit int, err error)

// FetchState says how fresh the protected branch tip is.
type FetchState string

// The fetch states of contract:ci.policy.
const (
	FetchFresh FetchState = "fresh"
	FetchStale FetchState = "stale"
	FetchNone  FetchState = "none"
)

var fullSHA = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)

// Branch is what the protected branch says.
type Branch struct {
	Name  string
	Tip   string
	Fetch FetchState
	// Assumed: origin's default branch could not be discovered, so Name is the spec fallback main.
	Assumed bool
	// Fetched: the fetch itself succeeded. With an empty Tip it means the fetched tip could not be read.
	Fetched bool
	Policy  map[string]json.RawMessage // the tip's flat policy; nil when it has none
}

type gitRunner struct {
	run  Runner
	root string
}

// gitScrub drops the variables that would point git at another repository, and
// --no-replace-objects keeps refs/replace from swapping the tree it reads.
var gitScrub = []string{"env", "-u", "GIT_DIR", "-u", "GIT_WORK_TREE", "-u", "GIT_INDEX_FILE", "-u", "GIT_OBJECT_DIRECTORY",
	"-u", "GIT_ALTERNATE_OBJECT_DIRECTORIES", "-u", "GIT_COMMON_DIR", "-u", "GIT_NAMESPACE", "-u", "GIT_CEILING_DIRECTORIES",
	"-u", "GIT_CONFIG_COUNT", "-u", "GIT_CONFIG_PARAMETERS", "GIT_TERMINAL_PROMPT=0", "git", "--no-replace-objects", "-c", "core.fsmonitor=false"}

// raw runs git bound to g.root and returns its output untouched.
func (g gitRunner) raw(ctx context.Context, args ...string) (string, bool) {
	return g.argv(ctx, append(append(slices.Clone(gitScrub), "-C", g.root), args...)...)
}

func (g gitRunner) argv(ctx context.Context, argv ...string) (string, bool) {
	out, exit, err := g.run(ctx, argv...)
	return out, err == nil && exit == 0
}

func (g gitRunner) git(ctx context.Context, args ...string) (string, bool) {
	out, ok := g.raw(ctx, args...)
	return strings.TrimSpace(out), ok
}

// lastLine is the answer of a one-line command that may have warned first.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// defaultBranch is origin's HEAD: the local symbolic ref, else the remote's own
// answer (one read-only ls-remote, for checkouts made by init+remote add+fetch),
// else main with assumed set. A name git would not accept is refused, never
// swapped for main.
func (g gitRunner) defaultBranch(ctx context.Context) (name string, assumed bool, err error) {
	out, ok := g.git(ctx, "symbolic-ref", "refs/remotes/origin/HEAD")
	if ok {
		name = strings.TrimPrefix(lastLine(out), "refs/remotes/origin/")
	} else if out, ok = g.git(ctx, "ls-remote", "--symref", "origin", "HEAD"); ok {
		for _, line := range strings.Split(out, "\n") {
			if rest, found := strings.CutPrefix(line, "ref: refs/heads/"); found {
				name, _, _ = strings.Cut(rest, "\t")
				break
			}
		}
	}
	if name == "" {
		return "main", true, nil
	}
	if strings.ContainsAny(name, " \t\r\n\x00") || strings.HasPrefix(name, "-") {
		return "", false, refuse("default branch name %q is not usable", name)
	}
	if _, ok := g.git(ctx, "check-ref-format", "refs/remotes/origin/"+name); !ok {
		return "", false, refuse("default branch name %q is not a valid ref", name)
	}
	return name, false, nil
}

// readBranch fetches the default branch and reads its policy at the tip.
func readBranch(ctx context.Context, run Runner, root string) (Branch, error) {
	g := gitRunner{run: run, root: root}
	origin, err := g.hasOrigin(ctx)
	if err != nil {
		return Branch{}, err
	}
	if !origin {
		return Branch{Fetch: FetchNone}, nil
	}
	name, assumed, err := g.defaultBranch(ctx)
	if err != nil {
		return Branch{}, err
	}
	b := Branch{Name: name, Assumed: assumed}
	ref := "refs/remotes/origin/" + b.Name
	// The scrub keeps a credential prompt from blocking the run; the Capture timeout bounds the rest.
	_, fetched := g.raw(ctx, "fetch", "--no-tags", "--no-recurse-submodules", "origin", "+refs/heads/"+b.Name+":"+ref)
	out, ok := g.git(ctx, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	tip := lastLine(out)
	if !ok || !fullSHA.MatchString(tip) {
		b.Fetch, b.Fetched = FetchNone, fetched // fetched but unreadable is told apart from never fetched
		return b, nil
	}
	b.Tip = tip
	b.Fetch = FetchStale
	if fetched {
		b.Fetch = FetchFresh
	}
	return b, nil
}

// policyAt reads the flat `policy:` block of .nself/ci.yaml at a commit.
func policyAt(ctx context.Context, run Runner, root, sha string) (map[string]json.RawMessage, error) {
	if !fullSHA.MatchString(sha) {
		return nil, refuse("pinned commit %q is not a commit id", sha)
	}
	g := gitRunner{run: run, root: root}
	// Paths are relative to root, which may be a subdirectory of the repository.
	listed, ok := g.git(ctx, "ls-tree", sha, "--", config.FileName)
	if !ok {
		return nil, refuse("cannot list %s at %s", config.FileName, sha[:12])
	}
	if listed == "" {
		if err := g.configDirIsPlain(ctx, sha); err != nil {
			return nil, err
		}
		return nil, nil // the commit has no config: no policy
	}
	if !strings.HasPrefix(listed, "100644 blob ") && !strings.HasPrefix(listed, "100755 blob ") {
		return nil, refuse("%s at %s is not a regular file", config.FileName, sha[:12])
	}
	out, ok := g.raw(ctx, "show", "--no-textconv", sha+":./"+config.FileName)
	if !ok {
		return nil, refuse("cannot read %s at %s", config.FileName, sha[:12])
	}
	if len(out) > config.MaxFileBytes {
		return nil, refuse("%s at %s is larger than %d bytes", config.FileName, sha[:12], config.MaxFileBytes)
	}
	loaded, err := config.Parse(sha[:12]+":"+config.FileName, []byte(out))
	if err != nil {
		return nil, err
	}
	return flatten(loaded.Config.Policy)
}

// flatten turns nested policy maps into dotted keys with JSON values. A key
// given twice (nested and dotted) or a null value is refused (E672).
func flatten(policy map[string]any) (map[string]json.RawMessage, error) {
	if len(policy) == 0 {
		return nil, nil
	}
	out := map[string]json.RawMessage{}
	var walk func(prefix string, m map[string]any) error
	walk = func(prefix string, m map[string]any) error {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if k == "" || strings.HasPrefix(k, ".") || strings.HasSuffix(k, ".") || strings.Contains(k, "..") {
				return refuse("invalid policy key %q", prefix+k)
			}
			full := prefix + k
			if sub, ok := m[k].(map[string]any); ok {
				if err := walk(full+".", sub); err != nil {
					return err
				}
				continue
			}
			if m[k] == nil {
				return refuse("%s has no value", full)
			}
			raw, err := json.Marshal(m[k])
			if err != nil {
				return refuse("%s: %v", full, err)
			}
			if _, dup := out[full]; dup {
				return refuse("%s is given twice", full)
			}
			out[full] = raw
		}
		return nil
	}
	if err := walk("", policy); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// worktreePolicy reads the `policy:` block of dir's .nself/ci.yaml: the
// revision scope, which may only tighten.
func worktreePolicy(dir string) (map[string]json.RawMessage, error) {
	if err := worktreeDirIsPlain(dir); err != nil {
		return nil, err
	}
	loaded, err := config.Load(filepath.Join(dir, config.FileName))
	if err != nil {
		if isNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("revision policy: %w", err)
	}
	return flatten(loaded.Config.Policy)
}
