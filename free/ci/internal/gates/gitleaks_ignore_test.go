package gates

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

const (
	fpGit   = "0123456789abcdef0123456789abcdef01234567" + ":internal/fixture.go:generic-api-key:12"
	fpNoGit = "testdata/fixture.env:aws-access-token:3"
)

func writeIgnore(t *testing.T, body string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, IgnoreFile), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestUnreviewedIgnoreFails(t *testing.T) {
	root := writeIgnore(t, "# Reviewed 2026-10-01 by sam: test fixture\n"+fpGit+"\n\n"+fpNoGit+"\n")
	err := ReviewIgnoreFile(root)
	var ie *IgnoreError
	if !errors.As(err, &ie) || ie.Code != "E812" {
		t.Fatalf("want E812, got %v", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "line 4 ") || strings.Contains(msg, "line 2") || strings.Contains(msg, "fixture") || strings.Contains(msg, "aws") {
		t.Fatalf("message must name only line 4: %s", msg)
	}
	c := model.Check{ID: "secrets.gitleaks", Result: "pass", Excerpt: "no leaks found"}
	ApplyIgnoreReview(root, &c)
	if c.Result != "fail" || c.FailureClass != "code" || !strings.HasPrefix(c.Excerpt, "E812:") {
		t.Fatalf("check not failed: %+v", c)
	}
}

func TestReviewedIgnorePasses(t *testing.T) {
	root := writeIgnore(t, "# header\n# Reviewed 2026-10-01: docs example\n"+fpGit+"\n"+fpNoGit+" # Reviewed 2026-10-02 by a b: fixture\n")
	if err := ReviewIgnoreFile(root); err != nil {
		t.Fatal(err)
	}
	if err := ReviewIgnoreFile(t.TempDir()); err != nil {
		t.Fatalf("missing file must pass: %v", err)
	}
	c := model.Check{ID: "secrets.gitleaks", Result: "pass"}
	ApplyIgnoreReview(root, &c)
	if c.Result != "pass" {
		t.Fatal(c)
	}
}

func TestIgnoreReviewEdges(t *testing.T) {
	cases := map[string][]int{
		"# Reviewed 2026-13-01: bad month\n" + fpNoGit + "\n":       {2},
		"# Reviewed 2026-10-01:\n" + fpNoGit + "\n":                 {2},
		"# Reviewed 2026-10-01: ok\n" + fpNoGit + "\n" + fpGit:      {3},
		fpNoGit + " # Reviewed 2026-10-01: ok\n" + fpGit + "\n":     {2},
		"# Reviewed 2026-10-01 by x: ok\n\n" + fpNoGit + "\n":       {3},
		"# Reviewed 2026-10-01 by x y: why\n" + fpGit + "\r\n# c\n": nil,
	}
	for body, want := range cases {
		got, err := UnreviewedLines(strings.NewReader(body))
		if err != nil || len(got) != len(want) || (len(want) == 1 && got[0] != want[0]) {
			t.Errorf("%q: got %v want %v", body, got, want)
		}
	}
	skip := model.Check{ID: "secrets.gitleaks", Result: "error"}
	ApplyIgnoreReview(writeIgnore(t, fpNoGit+"\n"), &skip)
	if skip.Result != "error" {
		t.Fatal("an errored check must stay an error")
	}
}

func TestFingerprintForms(t *testing.T) {
	for fp, want := range map[string][2]bool{
		fpGit:                         {true, true},
		fpNoGit:                       {true, false},
		"a:b:c:d:rule:7":              {true, false},
		"path:rule":                   {false, false},
		"path:rule:0":                 {false, false},
		"path:rule:x":                 {false, false},
		"/abs/path:rule:2":            {false, false},
		"path with space:rule:2":      {false, false},
		"path:rule:2#c":               {false, false},
		fpGit[:41] + ":generic-key:1": {false, false},
	} {
		v, c := FingerprintCommit(fp)
		if v != want[0] || c != want[1] {
			t.Errorf("%q: got %v %v", fp, v, c)
		}
	}
}

func TestAddReviewedIgnore(t *testing.T) {
	root := writeIgnore(t, "# existing\n"+fpGit)
	now := time.Date(2026, 10, 27, 23, 0, 0, 0, time.FixedZone("x", -5*3600))
	for i := 0; i < 2; i++ {
		added, err := AddReviewedIgnore(root, fpNoGit, "fixture value", "sam", now)
		if err != nil || added != (i == 0) {
			t.Fatalf("run %d: %v %v", i, added, err)
		}
	}
	if added, err := AddReviewedIgnore(root, fpGit, "dup", "", now); err != nil || added {
		t.Fatalf("existing entry must be a no-op: %v %v", added, err)
	}
	raw, _ := os.ReadFile(filepath.Join(root, IgnoreFile))
	want := "# existing\n" + fpGit + "\n# Reviewed 2026-10-28 by sam: fixture value\n" + fpNoGit + "\n"
	if string(raw) != want {
		t.Fatalf("got %q", raw)
	}
	if lines, _ := UnreviewedLines(strings.NewReader(string(raw))); len(lines) != 1 || lines[0] != 2 {
		t.Fatalf("only the pre-existing line is unreviewed: %v", lines)
	}
	for _, bad := range [][2]string{{"", ""}, {"a\nb", ""}, {"ok", "x:y"}} {
		if _, err := AddReviewedIgnore(root, fpNoGit+"0", bad[0], bad[1], now); err == nil {
			t.Errorf("accepted reason %q by %q", bad[0], bad[1])
		}
	}
}

func TestIgnoreSymlinkRefused(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.WriteFile(outside, []byte(fpNoGit+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, IgnoreFile)); err != nil {
		t.Skip("symlinks unavailable")
	}
	if err := ReviewIgnoreFile(root); err == nil || !strings.Contains(err.Error(), "E812") {
		t.Fatalf("symlink must be refused: %v", err)
	}
	if _, err := AddReviewedIgnore(root, fpGit, "r", "", time.Unix(0, 0)); err == nil {
		t.Fatal("write through symlink must be refused")
	}
}
