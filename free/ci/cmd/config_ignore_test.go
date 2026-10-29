package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/gates"
)

const (
	ignoreFPGit   = "0123456789abcdef0123456789abcdef01234567" + ":internal/fixture.go:generic-api-key:12"
	ignoreFPNoGit = "testdata/fixture.env:aws-access-token:3"
)

func ignoreTestDeps(root string) ignoreDeps {
	return ignoreDeps{
		Getenv: func(string) string { return "" },
		Getwd:  func() (string, error) { return root, nil },
		Now:    func() time.Time { return time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC) },
	}
}

func runIgnore(t *testing.T, root string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := runIgnoreAdd(args, &out, &errb, ignoreTestDeps(root))
	return code, out.String(), errb.String()
}

func readIgnore(t *testing.T, root string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, gates.IgnoreFile))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(raw)
}

// fakeSecrets are token shapes assembled at run time so no literal looks like a credential.
func fakeSecrets() []string {
	body := strings.Repeat("Ab3Xy9", 6)
	return []string{
		"gh" + "p_" + body,
		"AK" + "IA" + "QWERTYUIOP234567",
		"path/" + "gl" + "pat-" + body + ":rule:1",
	}
}

// strideSegment builds an n-byte mixed letter and digit segment of distinct
// bytes, high in entropy without a literal in the source.
func strideSegment(n int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = alphabet[(i*7)%len(alphabet)]
	}
	return string(b)
}

func TestIgnoreAddRefusesSecretShape(t *testing.T) {
	for _, secret := range fakeSecrets() {
		root := t.TempDir()
		code, out, errs := runIgnore(t, root, secret, "--reason", "fixture")
		if code != 1 || !strings.Contains(errs, "E812") || readIgnore(t, root) != "" {
			t.Fatalf("token shape not refused: exit %d out %q", code, out)
		}
	}
	root := t.TempDir()
	random := "cfg/" + strideSegment(24) + ".env:generic-api-key:4"
	if code, _, _ := runIgnore(t, root, random, "--reason", "x"); code != 1 || readIgnore(t, root) != "" {
		t.Fatalf("high-entropy segment not refused: exit %d", code)
	}
	if code, _, _ := runIgnore(t, root, ignoreFPNoGit, "--reason", "copied "+fakeSecrets()[0]); code != 1 {
		t.Fatal("token-shaped reason not refused")
	}
}

func TestIgnoreAddNeverEchoes(t *testing.T) {
	args := [][]string{
		{fakeSecrets()[0], "--reason", "r"},
		{fakeSecrets()[1], "--reason", "r", "--by", "sam"},
		{"-" + fakeSecrets()[0], "--reason", "r"},
		{"not-a-fingerprint-" + "value", "--reason", "r"},
		{ignoreFPGit, "--reason", "fixture", "--by", "sam"},
		{ignoreFPNoGit, "--reason", "fixture"},
		{ignoreFPNoGit, ignoreFPGit, "--reason", "two"},
		{ignoreFPNoGit},
	}
	for _, a := range args {
		root := t.TempDir()
		_, out, errs := runIgnore(t, root, a...)
		value := strings.TrimPrefix(a[0], "-")
		for _, frag := range []string{value, "fixture.go", "fingerprint-value", fakeSecrets()[0][4:]} {
			if strings.Contains(out+errs, frag) {
				t.Fatalf("output echoes the argument (%q): %q %q", frag, out, errs)
			}
		}
	}
}

func TestIgnoreAddIdempotent(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 2; i++ {
		if code, _, errs := runIgnore(t, root, ignoreFPGit, "--reason", "test fixture", "--by", "sam"); code != 0 {
			t.Fatalf("run %d: exit %d %s", i, code, errs)
		}
	}
	want := "# Reviewed 2026-10-27 by sam: test fixture\n" + ignoreFPGit + "\n"
	if got := readIgnore(t, root); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if err := gates.ReviewIgnoreFile(root); err != nil {
		t.Fatal(err)
	}
}

func TestIgnoreAddBothForms(t *testing.T) {
	root := t.TempDir()
	for fp, form := range map[string]string{ignoreFPGit: "git", ignoreFPNoGit: "no-git"} {
		code, out, errs := runIgnore(t, root, "--reason", "fixture", fp)
		if code != 0 || !strings.Contains(out, "added one reviewed") || !strings.Contains(out, "("+form+" form)") {
			t.Fatalf("%s form: exit %d %q %q", form, code, out, errs)
		}
	}
	got := readIgnore(t, root)
	if !strings.Contains(got, ignoreFPGit+"\n") || !strings.Contains(got, ignoreFPNoGit+"\n") {
		t.Fatalf("missing entries: %q", got)
	}
	if err := gates.ReviewIgnoreFile(root); err != nil {
		t.Fatal(err)
	}
}

func TestIgnoreAddUsage(t *testing.T) {
	root := t.TempDir()
	for _, a := range [][]string{{ignoreFPNoGit}, {}, {"--bogus", ignoreFPNoGit, "--reason", "r"}, {"src/config.go:rule:0", "--reason", "r"}} {
		if code, _, _ := runIgnore(t, root, a...); code == 0 {
			t.Errorf("%d args: accepted", len(a))
		}
	}
	if readIgnore(t, root) != "" {
		t.Fatal("a refused call wrote the file")
	}
	if _, ok := v15Subcommands["config ignore add"]; !ok {
		t.Fatal("config ignore add is not registered")
	}
}
