package main

// Purpose: the link confinement of the job-shim tree stream (D-1230): a stage-out tree whose link is
// absolute, climbs out with ../, or escapes only when a chain of its links is followed is refused, the
// step's hand-back fails and the agent's workspace is left empty; stage-in refuses the same links in the
// agent's workspace before the job user sees them; an in-tree link still crosses and works.
// Inputs:  the fake shim rig of job_shim_stage_test.go and hand-built trees. Constraints: no sudo.

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// unpackLinks packs a tree with the given links (name -> target) plus dirs and unpacks it into a new root.
func unpackLinks(t *testing.T, dirs []string, links [][2]string) (*os.Root, error) {
	t.Helper()
	src, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	for _, d := range dirs {
		if err := src.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := src.WriteFile("ok.txt", []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, l := range links {
		if err := src.Symlink(l[1], l[0]); err != nil {
			t.Fatal(err)
		}
	}
	pr, pw := io.Pipe()
	go func() { _ = pw.CloseWithError(packTree(src, pw)) }()
	dst, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dst.Close() })
	err = unpackTree(dst, pr)
	_, _ = io.Copy(io.Discard, pr)
	return dst, err
}

func TestUnpackRefusesLinksThatLeaveTheTree(t *testing.T) {
	cases := []struct {
		name  string
		dirs  []string
		links [][2]string
	}{
		{"absolute target", nil, [][2]string{{"key", "/var/lib/nself-ci-agent/node.key"}}},
		{"absolute directory", nil, [][2]string{{"etc", "/etc"}}},
		{"../ escape", []string{"a"}, [][2]string{{"a/up", "../../.."}}},
		{"../ escape through a sibling", []string{"a/b"}, [][2]string{{"a/b/up", "../../../x"}}},
		{"chain of links", []string{"deep/deeper"}, [][2]string{{"deep/deeper/s", "../.."}, {"deep/deeper/x", "s/.."}}},
		{"chain to an escaping link", []string{"d"}, [][2]string{{"d/top", ".."}, {"hop", "d/top/d/top/.."}}},
		{"loop", nil, [][2]string{{"l1", "l2"}, {"l2", "l1"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := unpackLinks(t, c.dirs, c.links); err == nil {
				t.Fatalf("a tree with %v unpacked", c.links)
			}
		})
	}
}

func TestUnpackKeepsInTreeLinks(t *testing.T) {
	dst, err := unpackLinks(t, []string{"a/b"}, [][2]string{
		{"same", "ok.txt"}, {"a/b/up", "../../ok.txt"}, {"a/dir", "b"}, {"a/via", "dir/up"}, {"dangling", "a/missing"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"same", "a/b/up", "a/via"} {
		got, err := dst.ReadFile(name)
		if err != nil || string(got) != "ok" {
			t.Fatalf("in-tree link %s: %q %v", name, got, err)
		}
	}
}

func TestStageOutWithAnEscapingLinkFailsAndLeavesNothing(t *testing.T) {
	rig := fakeShim(t)
	for i, target := range []string{"/etc", "../../../../..", ""} {
		attempt := "esc" + string(rune('a'+i))
		ws := t.TempDir()
		if err := os.WriteFile(filepath.Join(ws, "src.txt"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := (shimStager{user: "nself-ci-job-1"}).StageIn(context.Background(), attempt, ws); err != nil {
			t.Fatal(err)
		}
		jobWS := filepath.Join(rig.jobs, "1", attempt, "ws")
		if target == "" { // a chain: each link inside, the pair outside
			if err := os.MkdirAll(filepath.Join(jobWS, "p", "q"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("../..", filepath.Join(jobWS, "p", "q", "s")); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("s/..", filepath.Join(jobWS, "p", "q", "evil")); err != nil {
				t.Fatal(err)
			}
		} else if err := os.Symlink(target, filepath.Join(jobWS, "evil")); err != nil {
			t.Fatal(err)
		}
		if err := (shimStager{user: "nself-ci-job-1"}).StageOut(context.Background(), attempt, ws); err == nil {
			t.Fatalf("stage-out accepted a link to %q", target)
		}
		if entries, _ := os.ReadDir(ws); len(entries) != 0 {
			t.Fatalf("a refused stage-out left %v in the workspace", entries)
		}
	}
}

func TestStageInRefusesLinksThatLeaveTheWorkspace(t *testing.T) {
	rig := fakeShim(t)
	outside := t.TempDir()
	secret := filepath.Join(outside, "node.key")
	if err := os.WriteFile(secret, []byte("k"), 0o600); err != nil {
		t.Fatal(err)
	}
	for i, target := range []string{secret, outside, "../../outside"} {
		attempt := "in" + string(rune('a'+i))
		ws := t.TempDir()
		if err := os.Symlink(target, filepath.Join(ws, "key-link")); err != nil {
			t.Fatal(err)
		}
		if _, _, err := (shimStager{user: "nself-ci-job-1"}).StageIn(context.Background(), attempt, ws); err == nil {
			t.Fatalf("stage-in accepted a link to %q", target)
		}
		if _, err := os.Lstat(filepath.Join(rig.jobs, "1", attempt, "ws", "key-link")); err == nil {
			t.Fatalf("the link to %q reached the job copy", target)
		}
	}
	if info, _ := os.Stat(secret); info.Mode().Perm() != 0o600 {
		t.Fatalf("the hand-off changed the key's mode: %v", info.Mode())
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 1 {
		t.Fatalf("the hand-off wrote through a link: %v", entries)
	}
}
