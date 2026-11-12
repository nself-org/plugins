package main

// Purpose: P7-WIRE-10: `ci serve watch` refuses to start when the station's gh login is unreadable,
//          driven through the real argv entry (runV15) with a gh stub that fails.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWire10LoginFailureExits(t *testing.T) {
	base := t.TempDir()
	repo, home, bin := filepath.Join(base, "repo"), filepath.Join(base, "home"), filepath.Join(base, "bin")
	for _, d := range []string{repo, home, bin} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, argv := range [][]string{{"init", "-q"}, {"remote", "add", "origin", "https://github.com/acme/widgets.git"}} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, argv...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", argv, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("NSELF_CI_HOME", home)
	t.Setenv("NSELF_CI_REPO", repo)
	t.Chdir(repo)
	code, _, errs := capture(t, func() int { return runV15([]string{"serve", "watch", "--once"}) })
	if code != 1 {
		t.Fatalf("exit = %d, want 1; stderr: %s", code, errs)
	}
	if !strings.Contains(errs, "no trusted login") || !strings.Contains(errs, "login unreadable") {
		t.Fatalf("stderr does not name the login failure: %q", errs)
	}
}
