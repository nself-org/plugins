//go:build security && unix

package redteam

// Purpose: red team attacks on artifact collection (TM-14, TM-16): hostile files in the job
// workdir (symlinks, hard links, FIFOs, devices, traversal and .git declarations) and secrets
// hidden from the value scanner (compression, hex, split files).
// Inputs:  the real records.Collector over a real cas.Store in a temp dir; the real redact.Redactor.
// Outputs: pass or fail; a pass means the attack failed.
// Constraints: local only; the "outside" file lives in its own temp dir and holds a marker string.

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/redact"
)

type artEnv struct {
	t       *testing.T
	home    string
	ws      string
	outside string
	blobs   *cas.Store
	red     *redact.Redactor
	pol     records.Policy
}

const outsideMarker = "OUTSIDE-FILE-MARKER-must-never-be-stored"

func newArtEnv(t *testing.T) *artEnv {
	t.Helper()
	e := &artEnv{t: t, home: t.TempDir(), ws: filepath.Join(t.TempDir(), "ws"), outside: t.TempDir(), red: redact.New()}
	var err error
	if e.blobs, err = cas.New(e.home); err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(e.ws, 0o755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(e.outside, "secret.txt"), []byte(outsideMarker), 0o644); err != nil {
		t.Fatal(err)
	}
	e.pol = records.Policy{MaxFile: 1 << 20, MaxTotal: 64 << 20, ProjectShare: 50, StorePath: e.home,
		Statfs: func(string) (uint64, uint64, error) { return 100 << 30, 200 << 30, nil }}
	return e
}

func (e *artEnv) write(rel, content string) {
	e.t.Helper()
	p := filepath.Join(e.ws, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		e.t.Fatal(err)
	}
}

// collect runs the declared paths through Normalize and the Collector; the call is bounded.
func (e *artEnv) collect(paths ...string) ([]records.Staged, error) {
	e.t.Helper()
	var arts []model.ArtifactDecl
	for _, p := range paths {
		arts = append(arts, model.ArtifactDecl{Path: p})
	}
	ds, err := records.Normalize(model.Job{Artifacts: arts}, false, nil)
	if err != nil {
		return nil, err
	}
	type res struct {
		s   []records.Staged
		err error
	}
	ch := make(chan res, 1)
	go func() {
		s, err := records.Collector{Blobs: e.blobs, Scanner: e.red, Policy: e.pol}.Collect(context.Background(), "proj", e.ws, ds)
		ch <- res{s, err}
	}()
	select {
	case r := <-ch:
		return r.s, r.err
	case <-time.After(10 * time.Second):
		e.t.Fatalf("collect %v hung (FIFO or device read?)", paths)
		return nil, nil
	}
}

// storedBytes returns the concatenation of every artifact blob in the store.
func (e *artEnv) storedBytes() string {
	var b strings.Builder
	_ = filepath.Walk(filepath.Join(e.home, "blobs"), func(p string, info os.FileInfo, err error) error {
		if err == nil && info.Mode().IsRegular() {
			c, _ := os.ReadFile(p)
			b.Write(c)
		}
		return nil
	})
	return b.String()
}

func TestB_Artifacts_HostileWorkdirEntries(t *testing.T) {
	cases := map[string]func(e *artEnv) string{
		"symlink-to-outside-file": func(e *artEnv) string {
			_ = os.Symlink(filepath.Join(e.outside, "secret.txt"), filepath.Join(e.ws, "link"))
			return "link"
		},
		"relative-symlink-escape": func(e *artEnv) string {
			_ = os.Symlink("../../../../../../../../"+strings.TrimPrefix(filepath.Join(e.outside, "secret.txt"), "/"), filepath.Join(e.ws, "rel"))
			return "rel"
		},
		"dir-symlink-in-path": func(e *artEnv) string {
			_ = os.Symlink(e.outside, filepath.Join(e.ws, "dir"))
			return "dir/secret.txt"
		},
		"dir-symlink-glob": func(e *artEnv) string {
			_ = os.Symlink(e.outside, filepath.Join(e.ws, "dir"))
			return "dir/*"
		},
		"dir-symlink-as-declared-dir": func(e *artEnv) string {
			_ = os.Symlink(e.outside, filepath.Join(e.ws, "dir"))
			return "dir"
		},
		"fifo": func(e *artEnv) string {
			if err := syscall.Mkfifo(filepath.Join(e.ws, "pipe"), 0o644); err != nil {
				e.t.Fatal(err)
			}
			return "pipe"
		},
		"hardlink-to-outside": func(e *artEnv) string {
			if err := os.Link(filepath.Join(e.outside, "secret.txt"), filepath.Join(e.ws, "hard")); err != nil {
				e.t.Fatalf("hard link fixture: %v", err)
			}
			return "hard"
		},
		"dev-null-symlink": func(e *artEnv) string { _ = os.Symlink("/dev/zero", filepath.Join(e.ws, "z")); return "z" },
		"dotdot":           func(e *artEnv) string { return "../" + filepath.Base(e.outside) + "/secret.txt" },
		"absolute":         func(e *artEnv) string { return filepath.Join(e.outside, "secret.txt") },
		"dotgit-hook": func(e *artEnv) string {
			e.write(".git/hooks/pre-commit", "#!/bin/sh\n")
			return ".git/hooks/pre-commit"
		},
		"dotgit-upper": func(e *artEnv) string { e.write(".GIT/config", "x"); return ".GIT/config" },
		"dotgit-dots":  func(e *artEnv) string { e.write(".git./config", "x"); return ".git./config" },
		"dotgit-zwnj":  func(e *artEnv) string { return ".g‌it/config" },
		"git-short":    func(e *artEnv) string { return "GIT~1/config" },
		"backslash":    func(e *artEnv) string { return `a\..\..\x` },
		"bidi-name":    func(e *artEnv) string { return "report‮gpj.exe" },
		"nul-name":     func(e *artEnv) string { return "a\x00b" },
	}
	for name, mk := range cases {
		t.Run(name, func(t *testing.T) {
			e := newArtEnv(t)
			p := mk(e)
			staged, err := e.collect(p)
			if err == nil && len(staged) > 0 {
				t.Errorf("hostile entry %q was collected: %+v", p, staged)
			}
			if strings.Contains(e.storedBytes(), outsideMarker) {
				t.Errorf("bytes of the outside file reached the blob store")
			}
		})
	}
}

// A symlink swapped in for a regular file between planning and streaming is the race the Root
// handle exists for; the closest deterministic form is a directory replaced by a link before Collect.
func TestB_Artifacts_RootStaysPinned(t *testing.T) {
	e := newArtEnv(t)
	e.write("out/app", "payload")
	if err := os.RemoveAll(filepath.Join(e.ws, "out")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(e.outside, filepath.Join(e.ws, "out")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.outside, "app"), []byte(outsideMarker), 0o644); err != nil {
		t.Fatal(err)
	}
	if staged, err := e.collect("out/app"); err == nil && len(staged) > 0 {
		t.Errorf("a path through a swapped-in directory link was collected")
	}
	if strings.Contains(e.storedBytes(), outsideMarker) {
		t.Errorf("outside bytes stored")
	}
}

func TestB_Artifacts_SecretHiddenFromScanner(t *testing.T) {
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	_, _ = zw.Write([]byte(filler + "token=" + fakeSecret + filler))
	_ = zw.Close()
	forms := map[string]string{
		"hex":       hex.EncodeToString([]byte(fakeSecret)),
		"url-all":   percentAll(fakeSecret),
		"utf16le":   toUTF16LE(fakeSecret),
		"gzip":      gz.String(),
		"split":     fakeSecret[:12] + "\n" + fakeSecret[12:],
		"interleaf": strings.Join(strings.Split(fakeSecret, ""), "\x00"),
	}
	for name, content := range forms {
		e := newArtEnv(t)
		if err := e.red.Register([]byte(fakeSecret)); err != nil {
			t.Fatal(err)
		}
		e.write("out/f.bin", content)
		_, err := e.collect("out/f.bin")
		if err == nil {
			switch name {
			case "gzip", "utf16le", "interleaf":
				t.Logf("%s: collected", name)
				if !openFinding(t, "B06") {
					continue
				}
			}
			t.Errorf("%s-encoded secret was collected into an artifact", name)
		}
	}
}

func toUTF16LE(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		b.WriteByte(s[i])
		b.WriteByte(0)
	}
	return b.String()
}

func percentAll(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		b.WriteString(fmt.Sprintf("%%%02X", s[i]))
	}
	return b.String()
}
