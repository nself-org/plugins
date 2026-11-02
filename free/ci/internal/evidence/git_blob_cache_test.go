package evidence

// Purpose: prove cached object reads retain bytes, input digests and failure
// behavior while repeated/concurrent opens issue one cat-file per object.
// Inputs: committed binary, empty and lockfile fixtures. Outputs: equivalence
// and invocation assertions. Constraints: no worktree reads feed the digests.
// SPORT: setup git processes are outside the counted capture.

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestTreeFSBlobCachePreservesBytesAndIndependentReaders(t *testing.T) {
	body := "\x00binary\xff\n"
	dir, rev := newRepo(t, map[string]string{"a.bin": body, "b.bin": body, "empty": ""})
	var reads atomic.Int64
	capture := func(ctx context.Context, argv ...string) (string, int, error) {
		if strings.Contains(strings.Join(argv, " "), "cat-file blob") {
			reads.Add(1)
		}
		return capOrDefault(nil)(ctx, argv...)
	}
	tree, err := TreeFS(t.Context(), capture, dir, rev)
	ok(t, err)
	write(t, dir, "a.bin", "changed checkout")
	var wg sync.WaitGroup
	for i := range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			name, want := "a.bin", body
			if i%3 == 1 {
				name = "b.bin"
			} else if i%3 == 2 {
				name, want = "empty", ""
			}
			got, err := fs.ReadFile(tree, name)
			if err != nil || string(got) != want {
				t.Errorf("%s = %q, %v; want %q", name, got, err, want)
			}
		}()
	}
	wg.Wait()
	if reads.Load() != 2 {
		t.Fatalf("cat-file calls = %d; want one per distinct blob (2)", reads.Load())
	}
}

func TestTreeFSBlobCacheRetriesFailures(t *testing.T) {
	for _, bad := range []struct {
		name string
		out  string
		exit int
		err  error
	}{
		{name: "exit", exit: 1},
		{name: "error", err: fmt.Errorf("transient capture failure")},
		{name: "short", out: "x"},
	} {
		t.Run(bad.name, func(t *testing.T) {
			dir, rev := newRepo(t, map[string]string{"a": "bytes\n"})
			reads := 0
			capture := func(ctx context.Context, argv ...string) (string, int, error) {
				if strings.Contains(strings.Join(argv, " "), "cat-file blob") {
					reads++
					if reads == 1 {
						return bad.out, bad.exit, bad.err
					}
				}
				return capOrDefault(nil)(ctx, argv...)
			}
			tree, err := TreeFS(t.Context(), capture, dir, rev)
			ok(t, err)
			if _, err := fs.ReadFile(tree, "a"); err == nil {
				t.Fatal("failed/short object read succeeded")
			}
			for range 2 {
				got, err := fs.ReadFile(tree, "a")
				if err != nil || string(got) != "bytes\n" {
					t.Fatalf("retry = %q, %v", got, err)
				}
			}
			if reads != 2 {
				t.Fatalf("cat-file calls = %d; want failure and successful retry", reads)
			}
		})
	}
}

// uncachedObjects is the former Open behavior, retaining the same tree metadata.
type uncachedObjects struct{ *treeFS }

func (u uncachedObjects) Open(name string) (fs.File, error) {
	if b, exists := u.files[name]; exists {
		out, valid := git(u.ctx, u.c, u.root, "cat-file", "blob", b.oid)
		if !valid || int64(len(out)) != b.size {
			return nil, fmt.Errorf("unreadable object %s", b.oid)
		}
		return &memFile{name: name, r: strings.NewReader(out), size: b.size}, nil
	}
	return u.treeFS.Open(name)
}

func TestCachedObjectsProduceIdenticalInputDigests(t *testing.T) {
	dir, rev := newRepo(t, map[string]string{
		"go.mod": "module example.test/tiny\n", "go.sum": "lock bytes\n",
		"web/package-lock.json": "{}\n", ".nself/ci.yaml": "version: 1\n",
	})
	tree, err := TreeFS(t.Context(), nil, dir, rev)
	ok(t, err)
	before, err := InputsOf(uncachedObjects{tree.(*treeFS)})
	ok(t, err)
	after, err := InputsOf(tree)
	ok(t, err)
	a, err := json.Marshal(before)
	ok(t, err)
	b, err := json.Marshal(after)
	ok(t, err)
	if string(a) != string(b) {
		t.Fatalf("input digest bytes changed: before %s; after %s", a, b)
	}
}
