package cas

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStoreDirectoryErrorPaths(t *testing.T) {
	s := testStore(t)
	want := errors.New("injected directory error")
	priorMkdir := mkdirBlobDir
	mkdirBlobDir = func(string, os.FileMode) error { return want }
	t.Cleanup(func() { mkdirBlobDir = priorMkdir })
	if err := s.secureDir(s.blobs()); !errors.Is(err, want) {
		t.Fatalf("mkdir: %v", err)
	}
	mkdirBlobDir = priorMkdir
	if err := s.secureDir(s.blobs()); err != nil {
		t.Fatal(err)
	}
	if err := s.syncStoreDir(filepath.Join(t.TempDir(), "outside")); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid sync path: %v", err)
	}
	if err := s.syncStoreDir(filepath.Join(s.blobs(), "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing directory: %v", err)
	}
	priorSync := syncStoreFile
	syncStoreFile = func(*os.File) error { return want }
	t.Cleanup(func() { syncStoreFile = priorSync })
	if err := s.syncStoreDir(filepath.Join(s.blobs(), "tmp")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing tmp: %v", err)
	}
	if err := s.secureDir(filepath.Join(s.blobs(), "tmp")); err != nil {
		t.Fatal(err)
	}
	if err := s.syncStoreDir(filepath.Join(s.blobs(), "tmp")); !errors.Is(err, want) {
		t.Fatalf("sync: %v", err)
	}
}

func TestCheckDirRejectsBlobRootLink(t *testing.T) {
	s := testStore(t)
	r := CacheRealm("root-link")
	d := digestOf([]byte("x"))
	p, _ := s.blobPath(r, d)
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(outside, string(r), "sha256", d[:2]), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, s.blobs()); err != nil {
		t.Fatal(err)
	}
	if err := s.checkDir(filepath.Dir(p)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("blob root link accepted: %v", err)
	}
}

func TestSweepTempInfoAndRemoveFailures(t *testing.T) {
	for _, mode := range []string{"info", "remove"} {
		t.Run(mode, func(t *testing.T) {
			s := testStore(t)
			if err := s.secureDir(filepath.Join(s.blobs(), "tmp")); err != nil {
				t.Fatal(err)
			}
			p := filepath.Join(s.blobs(), "tmp", "orphan")
			if err := os.WriteFile(p, []byte("old"), 0600); err != nil {
				t.Fatal(err)
			}
			now := time.Now()
			if err := os.Chtimes(p, now.Add(-25*time.Hour), now.Add(-25*time.Hour)); err != nil {
				t.Fatal(err)
			}
			want := errors.New("injected prune error")
			if mode == "info" {
				s.afterTempReadDir = func() {
					if err := os.Remove(p); err != nil {
						t.Fatal(err)
					}
				}
			} else {
				prior := removeFile
				removeFile = func(_ *Store, path string) error {
					if path == p {
						return want
					}
					return prior(s, path)
				}
				t.Cleanup(func() { removeFile = prior })
			}
			err := s.Sweep(context.Background(), "cache", func(Realm, string) bool { return false }, now)
			if mode == "info" && !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("missing entry: %v", err)
			}
			if mode == "remove" && !errors.Is(err, want) {
				t.Fatalf("prune error: %v", err)
			}
		})
	}
}

func TestQuarantineRenameAndStatFailures(t *testing.T) {
	for _, mode := range []string{"rename", "stat"} {
		t.Run(mode, func(t *testing.T) {
			s := testStore(t)
			r := CacheRealm("quarantine-" + mode)
			d, _, err := s.Put(context.Background(), r, strings.NewReader("good"))
			if err != nil {
				t.Fatal(err)
			}
			p, _ := s.blobPath(r, d)
			if err := os.WriteFile(p, []byte("evil"), 0600); err != nil {
				t.Fatal(err)
			}
			if mode == "rename" {
				s.beforeQuarantineRename = func() error {
					dir := filepath.Join(s.blobs(), "corrupt")
					if err := os.Rename(dir, dir+"-old"); err != nil {
						return err
					}
					return os.WriteFile(dir, []byte("block"), 0600)
				}
			} else {
				s.afterQuarantineRename = func() {
					dir := filepath.Join(s.blobs(), "corrupt")
					entries, err := os.ReadDir(dir)
					if err != nil || len(entries) != 1 {
						t.Fatalf("corrupt entries: %v, %d", err, len(entries))
					}
					if err := os.Remove(filepath.Join(dir, entries[0].Name())); err != nil {
						t.Fatal(err)
					}
				}
			}
			_, err = s.Open(r, d)
			if mode == "stat" && !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("missing quarantined entry: %v", err)
			}
			if mode == "rename" && (err == nil || errors.Is(err, ErrDigestMismatch)) {
				t.Fatalf("rename failure hidden: %v", err)
			}
		})
	}
}

func TestPartialWriteFaults(t *testing.T) {
	want := errors.New("injected partial error")
	for _, mode := range []string{"write", "short", "sync", "offset-write", "offset-sync", "offset-rename"} {
		t.Run(mode, func(t *testing.T) {
			s := testStore(t)
			p, err := s.Partial(CacheRealm(mode), digestOf([]byte("data")), "writer")
			if err != nil {
				t.Fatal(err)
			}
			priorWrite, priorSync := writePartialFile, syncPartialFile
			priorOffsetWrite, priorOffsetSync := writeOffsetFile, syncOffsetFile
			t.Cleanup(func() {
				writePartialFile, syncPartialFile = priorWrite, priorSync
				writeOffsetFile, syncOffsetFile = priorOffsetWrite, priorOffsetSync
			})
			switch mode {
			case "write":
				writePartialFile = func(*os.File, []byte) (int, error) { return 0, want }
			case "short":
				writePartialFile = func(*os.File, []byte) (int, error) { return 1, nil }
			case "sync":
				syncPartialFile = func(*os.File) error { return want }
			case "offset-write":
				writeOffsetFile = func(*os.File, string) (int, error) { return 0, want }
			case "offset-sync":
				syncOffsetFile = func(*os.File) error { return want }
			case "offset-rename":
				s.beforeOffsetRename = func() error {
					if err := os.Remove(p.offsetPath); err != nil {
						return err
					}
					return os.Mkdir(p.offsetPath, 0700)
				}
			}
			_, err = p.Write([]byte("data"))
			if mode == "short" {
				if !errors.Is(err, io.ErrShortWrite) {
					t.Fatalf("short write: %v", err)
				}
				return
			}
			if mode == "offset-rename" {
				if err == nil {
					t.Fatal("rename error hidden")
				}
				return
			}
			if !errors.Is(err, want) {
				t.Fatalf("%s fault: %v", mode, err)
			}
		})
	}
}

func TestPartialDiscardFailures(t *testing.T) {
	want := errors.New("injected remove error")
	for _, mode := range []string{"data", "offset"} {
		t.Run(mode, func(t *testing.T) {
			s := testStore(t)
			p, err := s.Partial(CacheRealm(mode), digestOf([]byte("expected")), "writer")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := p.Write([]byte("wrong")); err != nil {
				t.Fatal(err)
			}
			target := p.data
			if mode == "offset" {
				target = p.offsetPath
			}
			s.beforeRemove = func(path string) error {
				if path == target {
					return want
				}
				return nil
			}
			if err := p.Finalize(); !errors.Is(err, want) || !errors.Is(err, ErrPartialConflict) {
				t.Fatalf("discard: %v", err)
			}
		})
	}
}
