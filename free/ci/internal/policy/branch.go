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

var branchName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)
var fullSHA = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)

// Branch is what the protected branch says.
type Branch struct {
	Name   string
	Tip    string
	Fetch  FetchState
	Policy map[string]json.RawMessage // the tip's flat policy; nil when it has none
}

type gitRunner struct {
	run  Runner
	root string
}

func (g gitRunner) git(ctx context.Context, args ...string) (string, bool) {
	argv := append([]string{"git", "-c", "core.fsmonitor=false", "-C", g.root}, args...)
	out, exit, err := g.run(ctx, argv...)
	return strings.TrimSpace(out), err == nil && exit == 0
}

// DefaultBranch is origin's HEAD, else main.
func (g gitRunner) defaultBranch(ctx context.Context) string {
	out, ok := g.git(ctx, "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
	name := strings.TrimPrefix(out, "origin/")
	if !ok || !branchName.MatchString(name) || strings.Contains(name, "..") || strings.HasSuffix(name, ".lock") {
		return "main"
	}
	return name
}

func (g gitRunner) hasOrigin(ctx context.Context) bool {
	out, ok := g.git(ctx, "remote")
	if !ok {
		return false
	}
	for _, r := range strings.Fields(out) {
		if r == "origin" {
			return true
		}
	}
	return false
}

// readBranch fetches the default branch and reads its policy at the tip.
func readBranch(ctx context.Context, run Runner, root string) (Branch, error) {
	g := gitRunner{run: run, root: root}
	if !g.hasOrigin(ctx) {
		return Branch{Fetch: FetchNone}, nil
	}
	b := Branch{Name: g.defaultBranch(ctx)}
	ref := "refs/remotes/origin/" + b.Name
	_, fetched := g.git(ctx, "fetch", "--no-tags", "origin", "+refs/heads/"+b.Name+":"+ref)
	tip, ok := g.git(ctx, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	if !ok || !fullSHA.MatchString(tip) {
		b.Fetch = FetchNone
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
	object := sha + ":" + config.FileName
	if _, ok := g.git(ctx, "cat-file", "-e", object); !ok {
		return nil, nil // no config at that commit: no policy
	}
	out, ok := g.git(ctx, "show", object)
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
	loaded, err := config.Load(filepath.Join(dir, config.FileName))
	if err != nil {
		if isNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("revision policy: %w", err)
	}
	return flatten(loaded.Config.Policy)
}
