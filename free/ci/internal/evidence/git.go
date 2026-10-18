package evidence

// Purpose: read git facts for evidence through the exec funnel: the root
// commit (repo id), the tracked files and blobs of a revision (as an fs.FS so
// the detectors read git objects, never the working tree), and the version.
// Inputs:  a run.Capture and a repository root. Outputs: facts and an fs.FS.
// Constraints: this package never imports os/exec; every git call is argv,
// never a shell; file contents are fetched lazily by blob id.

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/report"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

// Capture runs a process and returns merged output and its exit code.
type Capture = cirun.Capture

func capOrDefault(c Capture) Capture {
	if c != nil {
		return c
	}
	return report.Capture
}

func git(ctx context.Context, c Capture, root string, args ...string) (string, bool) {
	out, exit, err := capOrDefault(c)(ctx, append([]string{"git", "-c", "core.fsmonitor=false", "-C", root}, args...)...)
	return out, err == nil && exit == 0
}

// RepoID is `local:<sha256 of the first root commit>` (contract:ci.evidence);
// ok is false when the repository has no commit.
func RepoID(ctx context.Context, c Capture, root string) (string, bool) {
	out, ok := git(ctx, c, root, "rev-list", "--max-parents=0", "HEAD")
	roots := strings.Fields(out)
	if !ok || len(roots) == 0 {
		return "", false
	}
	sort.Strings(roots) // several root commits: the smallest id names the repository
	return "local:" + digestString(roots[0]), true
}

// GitVersion is the `git --version` number, "" when unknown.
func GitVersion(ctx context.Context, c Capture, root string) string {
	out, ok := git(ctx, c, root, "--version")
	if !ok {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(out), "git version"))
}

// CommitExists reports whether rev names a commit object in the repository.
func CommitExists(ctx context.Context, c Capture, root, rev string) bool {
	_, ok := git(ctx, c, root, "cat-file", "-e", rev+"^{commit}")
	return ok
}

// treeFS is the tracked tree of one revision as an fs.FS. Directories come
// from the path list; a file is read by blob id when it is opened.
type treeFS struct {
	ctx  context.Context
	c    Capture
	root string
	// files maps a path to its blob id and size; symlinks and submodules are kept out.
	files map[string]blob
	dirs  map[string][]string // directory -> child names
}

type blob struct {
	oid  string
	size int64
}

// TreeFS lists the files tracked at rev. A listing that cannot be read is an
// error: the caller must not treat an unreadable tree as an empty one.
func TreeFS(ctx context.Context, c Capture, root, rev string) (fs.FS, error) {
	out, ok := git(ctx, c, root, "ls-tree", "-r", "-z", "--long", rev)
	if !ok {
		return nil, fail(CodeWorktree, 2, "cannot list the files of %s", short(rev))
	}
	t := &treeFS{ctx: ctx, c: c, root: root, files: map[string]blob{}, dirs: map[string][]string{}}
	seen := map[string]bool{}
	for _, rec := range strings.Split(out, "\x00") {
		meta, name, found := strings.Cut(rec, "\t")
		f := strings.Fields(meta)
		if !found || len(f) != 4 || f[1] != "blob" || f[0] == "120000" { // regular files only
			continue
		}
		size, _ := strconv.ParseInt(f[3], 10, 64)
		t.files[name] = blob{oid: f[2], size: size}
		for dir, child := path.Dir(name), path.Base(name); ; dir, child = path.Dir(dir), path.Base(dir) {
			if key := dir + "\x00" + child; !seen[key] {
				seen[key] = true
				t.dirs[dir] = append(t.dirs[dir], child)
			}
			if dir == "." {
				break
			}
		}
	}
	return t, nil
}

func (t *treeFS) Open(name string) (fs.File, error) {
	b, ok := t.files[name]
	if !ok || !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(t.ctx), 60*time.Second)
	defer cancel()
	out, ok := git(ctx, t.c, t.root, "cat-file", "blob", b.oid)
	if !ok || int64(len(out)) != b.size {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fmt.Errorf("git blob %s unreadable", short(b.oid))}
	}
	return &memFile{name: name, r: strings.NewReader(out), size: b.size}, nil
}

func (t *treeFS) Stat(name string) (fs.FileInfo, error) {
	if b, ok := t.files[name]; ok {
		return info{name: path.Base(name), size: b.size}, nil
	}
	if _, ok := t.dirs[name]; ok {
		return info{name: path.Base(name), dir: true}, nil
	}
	return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
}

func (t *treeFS) ReadDir(name string) ([]fs.DirEntry, error) {
	kids, ok := t.dirs[name]
	if !ok {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}
	sort.Strings(kids)
	out := make([]fs.DirEntry, 0, len(kids))
	for _, k := range kids {
		full := path.Join(name, k)
		if b, isFile := t.files[full]; isFile {
			out = append(out, fs.FileInfoToDirEntry(info{name: k, size: b.size}))
		} else {
			out = append(out, fs.FileInfoToDirEntry(info{name: k, dir: true}))
		}
	}
	return out, nil
}

type info struct {
	name string
	size int64
	dir  bool
}

func (i info) Name() string { return i.name }
func (i info) Size() int64  { return i.size }
func (i info) Mode() fs.FileMode {
	if i.dir {
		return fs.ModeDir | 0o555
	}
	return 0o444
}
func (i info) ModTime() time.Time { return time.Time{} }
func (i info) IsDir() bool        { return i.dir }
func (i info) Sys() any           { return nil }

type memFile struct {
	name string
	r    *strings.Reader
	size int64
}

func (f *memFile) Read(p []byte) (int, error) { return f.r.Read(p) }
func (f *memFile) Close() error               { return nil }
func (f *memFile) Stat() (fs.FileInfo, error) {
	return info{name: path.Base(f.name), size: f.size}, nil
}

var _ io.Reader = (*memFile)(nil)

func short(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}
