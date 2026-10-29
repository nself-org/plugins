package offload

// Purpose: turn a dirty working tree into a revision (P7-AUTO-13): a snapshot
// commit of the working tree of every path the user's index tracks (staged or
// not, including a file added and not yet committed) plus untracked files git
// does not ignore, parent HEAD, held under the private ref refs/nself-ci/offload/<id> so the NODE-03
// bundle transport can carry it.
// Inputs:  the repository root, a scratch dir and an id.
// Outputs: a Snapshot (sha, ref, names excluded) and a Release that deletes
// the ref.
// Constraints: uses a temporary index under scratch; never touches the user's
// index, refs/heads or working tree. Git runs with no GIT_DIR, no replace
// objects, hooks and fsmonitor off. Untracked secret-shaped or oversized files
// are excluded and named only; a tracked one refuses the offload (E807).

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// CodeTracked is raised when a tracked file is secret-shaped or too large.
const CodeTracked = "E807"

// RefPrefix is the private namespace of snapshot refs.
const RefPrefix = "refs/nself-ci/offload/"

// Capture runs a process and returns merged output and its exit code.
type Capture func(ctx context.Context, argv ...string) (string, int, error)

// Snapshot is the revision a dirty run executes.
type Snapshot struct {
	SHA      string   // the snapshot commit (HEAD when the tree is clean)
	Ref      string   // private ref; empty when the tree is clean
	Clean    bool     // true: the existing HEAD path applies unchanged
	Excluded []string // untracked paths left out, by name only
}

var idRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// Take builds the snapshot of root. A clean tree returns HEAD unchanged.
func Take(ctx context.Context, capture Capture, root, scratch, id string) (*Snapshot, error) {
	if !idRe.MatchString(id) {
		return nil, fmt.Errorf("offload: bad snapshot id")
	}
	g := func(index string, args ...string) (string, error) {
		argv := []string{"env", "-u", "GIT_DIR", "-u", "GIT_WORK_TREE", "-u", "GIT_INDEX_FILE"}
		if index != "" {
			argv = append(argv, "GIT_INDEX_FILE="+index)
		}
		// GIT_OPTIONAL_LOCKS=0: `status` must not refresh the user's index; GIT_LITERAL_PATHSPECS=1: a
		// file name is a path, never a pathspec pattern or magic.
		argv = append(argv, "GIT_OPTIONAL_LOCKS=0", "GIT_LITERAL_PATHSPECS=1", "GIT_AUTHOR_NAME=nself-ci", "GIT_AUTHOR_EMAIL=ci@localhost", "GIT_COMMITTER_NAME=nself-ci", "GIT_COMMITTER_EMAIL=ci@localhost",
			"git", "--no-replace-objects", "-c", "core.fsmonitor=false", "-c", "core.hooksPath=/dev/null", "-C", root)
		out, code, err := capture(ctx, append(argv, args...)...)
		if err != nil || code != 0 {
			return out, fmt.Errorf("offload: git %s failed", args[0])
		}
		return strings.TrimSpace(out), nil
	}
	head, err := g("", "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	status, err := g("", "status", "--porcelain", "--untracked-files=normal")
	if err != nil {
		return nil, err
	}
	if status == "" {
		return &Snapshot{SHA: head, Clean: true}, nil
	}
	tracked, err := g("", "ls-files", "-z")
	if err != nil {
		return nil, err
	}
	for _, p := range split(tracked) {
		if SecretPath(p) || tooBig(filepath.Join(root, p)) {
			return nil, fmt.Errorf("%s: tracked file %q is secret-shaped or over the size cap; the offload is refused", CodeTracked, p)
		}
	}
	others, err := g("", "ls-files", "-z", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	var keep []byte
	snap := &Snapshot{}
	for _, p := range split(others) {
		if SecretPath(p) || tooBig(filepath.Join(root, p)) {
			snap.Excluded = append(snap.Excluded, p)
			continue
		}
		keep = append(append(keep, p...), 0)
	}
	if err := os.MkdirAll(scratch, 0o700); err != nil {
		return nil, err
	}
	index, list := filepath.Join(scratch, "index-"+id), filepath.Join(scratch, "paths-"+id)
	defer os.Remove(index)
	defer os.Remove(list)
	if _, err := g(index, "read-tree", head); err != nil {
		return nil, err
	}
	inHead, err := g(index, "ls-files", "-z")
	if err != nil {
		return nil, err
	}
	keep = append(keep, stagedNew(root, split(tracked), split(inHead))...)
	if err := os.WriteFile(list, keep, 0o600); err != nil {
		return nil, err
	}
	if _, err := g(index, "add", "-u"); err != nil {
		return nil, err
	}
	if len(keep) > 0 {
		if _, err := g(index, "add", "--pathspec-from-file="+list, "--pathspec-file-nul"); err != nil {
			return nil, err
		}
	}
	tree, err := g(index, "write-tree")
	if err != nil {
		return nil, err
	}
	sha, err := g(index, "commit-tree", tree, "-p", head, "-m", "nself-ci offload snapshot "+id)
	if err != nil {
		return nil, err
	}
	snap.Ref = RefPrefix + id
	if _, err := g("", "update-ref", snap.Ref, sha); err != nil {
		return nil, err
	}
	snap.SHA = sha
	return snap, nil
}

// Release deletes the private ref. It is safe to call on success, failure and
// cancel, with a cancelled context's replacement, and more than once.
func Release(ctx context.Context, capture Capture, root string, s *Snapshot) error {
	if s == nil || s.Ref == "" {
		return nil
	}
	_, code, err := capture(ctx, "env", "-u", "GIT_DIR", "-u", "GIT_WORK_TREE", "git", "-C", root, "update-ref", "-d", s.Ref)
	if err != nil || code != 0 {
		return fmt.Errorf("offload: cannot delete the private ref")
	}
	s.Ref = ""
	return nil
}

// stagedNew lists, NUL-terminated, the paths the user's index tracks that HEAD
// does not (a file added with `git add` and not committed) and that exist in
// the working tree: `add -u` over a HEAD index never sees them.
func stagedNew(root string, tracked, inHead []string) []byte {
	head := make(map[string]bool, len(inHead))
	for _, p := range inHead {
		head[p] = true
	}
	var out []byte
	for _, p := range tracked {
		if head[p] {
			continue
		}
		head[p] = true // an unmerged path is listed once per stage
		if _, err := os.Lstat(filepath.Join(root, p)); err == nil {
			out = append(append(out, p...), 0)
		}
	}
	return out
}

func split(z string) []string {
	var out []string
	for _, p := range strings.Split(z, "\x00") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func tooBig(p string) bool {
	st, err := os.Lstat(p)
	return err == nil && st.Mode().IsRegular() && st.Size() > MaxFileBytes
}
