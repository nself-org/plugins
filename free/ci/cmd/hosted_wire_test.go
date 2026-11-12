package main

// Purpose: P7-WIRE-04. A bare `nself ci --prefer prefer-included` in a repository with a github.com origin
//          keeps the hosted runner list (the providers contributor looks up a credential, `gh auth token`, only
//          when a hosted runner can be placed); a private repository, or one whose visibility is unknown, keeps
//          NoHosted (D-1382) and the contributor never runs.
// Inputs:  the real binary built from this module, driven by argv in a throwaway git repository with a
//          github origin; a stub `gh` on PATH answers `api repos/..` (visibility) and logs every call.
// Constraints: no network (the stub gh answers; the credential lookup fails, so nothing is described); nothing in
//          this package is called or replaced (epic rule W1).

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// wire04Run builds the binary, runs a bare `ci --prefer prefer-included` in a repo whose origin is origin and
// whose stub gh reports visibility, and returns the gh calls the run made.
func wire04Run(t *testing.T, origin, visibility string) []string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "nself-ci")
	b := exec.Command("go", "build", "-o", bin, ".")
	b.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := b.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v: %s", err, out)
	}
	repo := filepath.Join(dir, "repo")
	if err := os.MkdirAll(filepath.Join(repo, ".nself"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".nself", "ci.yaml"), []byte("version: 1\njobs:\n  unit:\n    kind: test\n    run: [\"sh\", \"-c\", \"exit 0\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, a := range [][]string{{"init", "-q"}, {"remote", "add", "origin", origin}, {"add", "-A"}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "init"}} {
		c := exec.Command("git", append([]string{"-C", repo}, a...)...)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", a, err, out)
		}
	}
	log := filepath.Join(dir, "gh.log")
	bindir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bindir, 0o755); err != nil {
		t.Fatal(err)
	}
	gh := "#!/bin/sh\necho \"$*\" >> '" + log + "'\nif [ \"$1\" = api ]; then echo '" + visibility + "'; exit 0; fi\nexit 1\n"
	if err := os.WriteFile(filepath.Join(bindir, "gh"), []byte(gh), 0o755); err != nil {
		t.Fatal(err)
	}
	c := exec.Command(bin, "--prefer", "prefer-included")
	c.Dir = repo
	c.Env = append(os.Environ(), "NSELF_V15=1", "NSELF_CI_HOME="+filepath.Join(dir, "home"), "NSELF_CI_STORE_MIN_FREE=1B", "PATH="+bindir+string(os.PathListSeparator)+os.Getenv("PATH"))
	_ = c.Run() // the run's own result is not the subject; the gh calls are
	data, _ := os.ReadFile(log)
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

func wire04Has(calls []string, prefix string) bool {
	for _, c := range calls {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

func TestWire04BareRunBindsProviderRepo(t *testing.T) {
	calls := wire04Run(t, "https://github.com/acme/pub.git", "public")
	if !wire04Has(calls, "api repos/acme/pub") {
		t.Fatalf("visibility was not read for the origin repo: %v", calls)
	}
	if !wire04Has(calls, "auth token") {
		t.Fatalf("a public github origin kept NoHosted: no hosted credential lookup happened: %v", calls)
	}
}

func TestWire04PrivateRepoHostedRefused(t *testing.T) {
	for _, vis := range []string{"private", "internal", ""} {
		calls := wire04Run(t, "git@github.com:acme/secret.git", vis)
		if wire04Has(calls, "auth token") {
			t.Fatalf("visibility %q reached the hosted provider (D-1382): %v", vis, calls)
		}
	}
}
