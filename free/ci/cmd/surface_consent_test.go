package main

// Purpose: TestConsentNoPrompt (P7-CI-62, CI D31): every remote or destructive command of commands.yaml runs with
//          NSELF_NONINTERACTIVE=1 on a fixture store, the way a machine caller starts it after its consent, and must
//          neither prompt nor hang (10 s).
// Inputs:  the spec rows; a fixture HOME, state home and repository; stub ssh/scp/gh/docker/curl that fail at once.
// Constraints: no network (PATH holds only the stubs), nothing outside t.TempDir() is written; stdin is a pipe nobody
//          writes to, so a read of it blocks and is reported as a prompt; a prompt marker in the output also fails.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const consentBound = 10 * time.Second

var promptMarkers = []string{"[y/n]", "(y/n)", "type yes", "password:", "passphrase:", "press enter", "continue?"}

// consentCase is how a remote or destructive command is started and what proves it reached its own code. code is the
// exit status the command must end with and marker text only that code prints; a daemon (a listener by nature) must
// still be running after the short bound with no prompt. {repo} and {state} expand to the fixture directories.
type consentCase struct {
	args   []string
	code   int
	marker string
	daemon bool
	quiet  bool // success prints nothing: the exit status 0 of a command that validated its arguments is the proof
}

var consentCases = map[string]consentCase{
	"artifacts":             {[]string{"prune", "--dry-run"}, 0, "would remove", false, false},
	"build":                 {[]string{"{repo}/android"}, 2, "Android lane needs a git repository", false, false},
	"netproxy":              {nil, 2, "E679", false, false},
	"nodes add":             {[]string{"nosuch"}, 2, "E656", false, false},
	"nodes drain":           {[]string{"nosuch"}, 1, "E650", false, false},
	"nodes enable":          {[]string{"nosuch"}, 1, "E650", false, false},
	"nodes forget-project":  {[]string{"{repo}"}, 0, "forgot deploy-host exclusion", false, false},
	"nodes label":           {[]string{"nosuch", "a=b"}, 1, "E650", false, false},
	"nodes pair":            {[]string{"--enroll-addr", "127.0.0.1:0"}, 0, "Pairing window open", true, false},
	"nodes projects add":    {[]string{"nosuch", "p"}, 1, "E650", false, false},
	"nodes projects remove": {[]string{"nosuch", "p"}, 1, "E650", false, false},
	"nodes provision":       {[]string{"--github-url", "https://github.com/o/r"}, 1, "registration token is required", false, false},
	"nodes remove":          {[]string{"nosuch"}, 1, "E650", false, false},
	"nodes revoke":          {[]string{"nosuch"}, 1, "E650", false, false},
	"nodes rotate":          {[]string{"nosuch"}, 1, "E650", false, false},
	"nodes secrets grant":   {[]string{"nosuch", "--class", "project"}, 1, "E650", false, false},
	"nodes secrets revoke":  {[]string{"nosuch", "--class", "project"}, 1, "E650", false, false},
	"nodes set":             {[]string{"nosuch", "--reserve-cpu", "1"}, 1, "E650", false, false},
	"nodes test":            {[]string{"nosuch"}, 1, "E650", false, false},
	"nodes upgrade":         {[]string{"nosuch", "--version", "1.5.0"}, 1, "E650", false, false},
	"policy protect":        {[]string{"--repo", "o/r", "--dry-run"}, 2, "gh api failed", false, false},
	"providers authorize":   {[]string{"github", "--project", "{repo}"}, 1, "cannot loosen policy", false, false},
	"providers revoke":      {[]string{"github", "--project", "{repo}"}, 0, "nothing to revoke", false, false},
	"providers test":        {[]string{"nosuch"}, 1, "unknown provider", false, false},
	"run":                   {[]string{"nosuch"}, 0, "searching nosuch", false, false},
	"runs approve":          {[]string{"nosuch"}, 1, "E688", false, false},
	"runs cancel":           {[]string{"nosuch"}, 1, "E626", false, false},
	"runs prune":            {[]string{"--dry-run"}, 0, "would remove", false, false},
	"runs reproduce":        {[]string{"nosuch"}, 1, "E631", false, false},
	"runs rerun":            {[]string{"nosuch"}, 1, "E626", false, false},
	"runs resolve":          {[]string{"nosuch", "--idempotency-key", "k", "--abandon"}, 1, "E716", false, false},
	"serve admin roles":     {[]string{"list", "--json"}, 0, "serve admin roles", false, false},
	"serve admin token":     {[]string{"list", "--json"}, 0, "serve admin token", false, false},
	"serve restore":         {[]string{"--in", "{state}/none", "--kek", "{state}/none"}, 1, "E668", false, false},
	"serve uninstall":       {[]string{"--launchd", "--user", "--binary", "{state}/nself"}, 0, "", false, true},
	"serve watch":           {[]string{"--once", "--repos", "o/r"}, 1, "has no local checkout", false, false},
}

// runBounded starts name with args, a stdin nobody writes to and combined output; hung is true when it outlives the
// bound (it is then killed). A command that reads stdin therefore shows up as hung.
func runBounded(t *testing.T, bound time.Duration, name string, args ...string) (code int, out string, hung bool) {
	t.Helper()
	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdinR.Close()
	defer stdinW.Close()
	ctx, cancel := context.WithTimeout(context.Background(), bound)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "NSELF_CI_AS_BINARY=1")
	cmd.Stdin = stdinR
	cmd.WaitDelay = time.Second
	b, _ := cmd.CombinedOutput()
	return cmd.ProcessState.ExitCode(), string(b), ctx.Err() != nil
}

// promptIn returns the prompt marker found in out, or "".
func promptIn(out string) string {
	low := strings.ToLower(out)
	for _, m := range promptMarkers {
		if strings.Contains(low, m) {
			return m
		}
	}
	return ""
}

// TestConsentDetectorCatchesPromptAndHang: the harness fails a command that reads stdin or prints a question.
func TestConsentDetectorCatchesPromptAndHang(t *testing.T) {
	_, _, hung := runBounded(t, 300*time.Millisecond, "/bin/sh", "-c", "read answer")
	if !hung {
		t.Fatal("a command blocked on stdin was not reported")
	}
	_, out, hung := runBounded(t, consentBound, "/bin/sh", "-c", "echo 'Delete everything? [y/N]' >&2; exit 1")
	if hung || promptIn(out) == "" {
		t.Fatalf("a printed prompt was not reported: hung=%v out=%q", hung, out)
	}
}

func TestConsentNoPrompt(t *testing.T) {
	s := loadSpec(t)
	for _, p := range consentProblems(s) {
		t.Error(p)
	}
	stubs := t.TempDir()
	for _, name := range []string{"ssh", "scp", "gh", "docker", "curl"} {
		if err := os.WriteFile(filepath.Join(stubs, name), []byte("#!/bin/sh\nexit 255\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	home, state, repo := t.TempDir(), t.TempDir(), t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, "android"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubs+string(os.PathListSeparator)+"/usr/bin"+string(os.PathListSeparator)+"/bin")
	t.Setenv("HOME", home)
	t.Setenv("NSELF_CI_HOME", state)
	t.Setenv("NSELF_CI_REPO", repo)
	t.Setenv("NSELF_V15", "1")
	t.Setenv("NSELF_NONINTERACTIVE", "1")
	t.Setenv("NSELF_INVOKED_BY", "mcp")
	t.Setenv("NSELF_CLI_VERSION", "1.5.0")
	expand := func(a []string) []string {
		var out []string
		for _, v := range a {
			out = append(out, strings.NewReplacer("{repo}", repo, "{state}", state).Replace(v))
		}
		return out
	}
	covered := map[string]bool{}
	for _, r := range s.Commands {
		if r.SideEffect != "remote" && r.SideEffect != "destructive" {
			continue
		}
		c, ok := consentCases[r.Path]
		if !ok {
			t.Errorf("%s: no consentCases entry: say how to start it and what proves it ran", r.Path)
			continue
		}
		covered[r.Path] = true
		argv := append(strings.Fields(r.Path), expand(c.args)...)
		if r.Confirm != nil {
			for _, f := range r.Confirm.Flags {
				argv = append(argv, "--"+f)
			}
		}
		t.Run(r.Path, func(t *testing.T) {
			bound := consentBound
			if c.daemon {
				bound = 2 * time.Second
			}
			code, out, hung := runBounded(t, bound, os.Args[0], argv...)
			if m := promptIn(out); m != "" {
				t.Errorf("`nself ci %s` prompts (%q): %.200s", strings.Join(argv, " "), m, out)
			}
			switch {
			case c.daemon && !hung:
				t.Errorf("`nself ci %s` should keep running (a listener) but ended with %d: %.200s", strings.Join(argv, " "), code, out)
			case !c.daemon && hung:
				t.Fatalf("`nself ci %s` did not finish in %s (a prompt or a hang)", strings.Join(argv, " "), bound)
			case !c.daemon && code != c.code:
				t.Errorf("`nself ci %s` ended with %d, want %d: %.200s", strings.Join(argv, " "), code, c.code, out)
			}
			if c.marker == "" && !c.quiet {
				t.Errorf("consentCases entry has no marker: name the text only the command's own code prints")
			}
			if !strings.Contains(out, c.marker) {
				t.Errorf("`nself ci %s` never reached its own code (no %q): %.200s", strings.Join(argv, " "), c.marker, out)
			}
		})
	}
	for key := range consentCases {
		if !covered[key] {
			t.Errorf("consentCases entry %q matches no remote or destructive row", key)
		}
	}
}
